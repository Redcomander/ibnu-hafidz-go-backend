package main

import (
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"

	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/database"
	"github.com/ibnu-hafidz/web-v2/internal/handlers"
	"github.com/ibnu-hafidz/web-v2/internal/middleware"
	"github.com/ibnu-hafidz/web-v2/internal/models"
)

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load config
	cfg := config.Load()
	if cfg.Environment == "production" && (cfg.JWTSecret == "change-me-in-production" || cfg.JWTRefreshSecret == "change-me-refresh-secret") {
		log.Fatal("Refusing to start in production with default JWT secrets")
	}

	// Connect database
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate models
	if err := database.Migrate(db); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	// Migrate Notification
	if err := db.AutoMigrate(&models.Notification{}); err != nil {
		log.Fatalf("Failed to migrate notifications: %v", err)
	}
	if err := db.AutoMigrate(&models.UserActivityLog{}); err != nil {
		log.Fatalf("Failed to migrate activity logs: %v", err)
	}
	if err := db.AutoMigrate(&models.FeatureFlag{}); err != nil {
		log.Fatalf("Failed to migrate feature flags: %v", err)
	}
	// Migrate Lesson
	if err := db.AutoMigrate(&models.Lesson{}, &models.DiniyyahLesson{}); err != nil {
		log.Fatalf("Failed to migrate lessons: %v", err)
	}

	// Migrate Schedules
	if err := db.AutoMigrate(&models.Schedule{}, &models.DiniyyahSchedule{}); err != nil {
		log.Fatalf("Failed to migrate schedules: %v", err)
	}

	// Migrate Attendance (Absensi)
	if err := db.AutoMigrate(
		&models.Absensi{},
		&models.AbsensiDiniyyah{},
		&models.TeacherAttendance{},
		&models.SubstituteLog{},
	); err != nil {
		log.Fatalf("Failed to migrate attendance: %v", err)
	}

	// Migrate Arrival tracking
	if err := db.AutoMigrate(
		&models.StudentArrival{},
		&models.UserArrival{},
		&models.StudentArrivalSetting{},
		&models.SidebarMenuSetting{},
	); err != nil {
		log.Fatalf("Failed to migrate arrivals: %v", err)
	}

	// Migrate Halaqoh
	if err := db.AutoMigrate(
		&models.HalaqohAssignment{},
		&models.HalaqohAttendance{},
		&models.HalaqohTeacherAttendance{},
		&models.HalaqohSubstituteLog{},
	); err != nil {
		log.Fatalf("Failed to migrate halaqoh: %v", err)
	}

	// Migrate Laundry
	if err := db.AutoMigrate(
		&models.LaundryVendor{},
		&models.LaundryAccount{},
		&models.LaundryTransaction{},
		&models.LaundryLog{},
	); err != nil {
		log.Fatalf("Failed to migrate laundry: %v", err)
	}

	// Migrate Absensi Ekstra
	if err := db.AutoMigrate(
		&models.AbsensiEkstraGroup{},
		&models.AbsensiEkstraGroupStudent{},
		&models.AbsensiEkstraGroupSupervisor{},
		&models.AbsensiEkstraSession{},
		&models.AbsensiEkstraSessionRecord{},
	); err != nil {
		log.Fatalf("Failed to migrate absensi ekstra: %v", err)
	}

	// Migrate Gallery & Article
	if err := db.AutoMigrate(
		&models.Album{},
		&models.Gallery{},
		&models.Category{},
		&models.Tag{},
		&models.Article{},
		&models.ArticleView{},
		&models.Comment{},
	); err != nil {
		log.Printf("Warning: gallery/article migration: %v", err)
	}

	// Migrate Prestasi
	if err := db.AutoMigrate(&models.Prestasi{}); err != nil {
		log.Printf("Warning: prestasi migration: %v", err)
	}

	// Drop 'name' column if it exists (cleanup from previous migration artifact)
	// We ignore the error in case it's already dropped
	// Drop 'name' column if it exists (cleanup from previous migration artifact)
	// We ignore the error in case it's already dropped
	db.Exec("ALTER TABLE lessons DROP COLUMN name")

	// FORCE FIX: Ensure time columns are TIME type, not DATETIME
	// This resolves "Incorrect datetime value: '08:00:00'" error
	db.Exec("ALTER TABLE jadwal_formal MODIFY COLUMN jam_mulai TIME")
	db.Exec("ALTER TABLE jadwal_formal MODIFY COLUMN jam_selesai TIME")
	db.Exec("ALTER TABLE jadwal_diniyyahs MODIFY COLUMN jam_mulai TIME")
	db.Exec("ALTER TABLE jadwal_diniyyahs MODIFY COLUMN jam_selesai TIME")

	// Cleanup: remove orphaned created_by_id / submitted_by_id columns from failed migration
	db.Exec("ALTER TABLE absensi_ekstra_sessions DROP COLUMN created_by_id")
	db.Exec("ALTER TABLE absensi_ekstra_groups DROP COLUMN created_by_id")
	db.Exec("ALTER TABLE absensi_ekstra_session_records DROP COLUMN submitted_by_id")
	// Also drop stale foreign key constraints if they exist
	db.Exec("ALTER TABLE absensi_ekstra_sessions DROP FOREIGN KEY fk_absensi_ekstra_sessions_created_by")
	db.Exec("ALTER TABLE absensi_ekstra_sessions DROP FOREIGN KEY IF EXISTS fk_absensi_ekstra_sessions_created_by")
	// Also drop waktu columns that GORM may add as datetime instead of time
	db.Exec("ALTER TABLE absensi_ekstra_sessions MODIFY COLUMN waktu_mulai TIME")
	db.Exec("ALTER TABLE absensi_ekstra_sessions MODIFY COLUMN waktu_selesai TIME")

	// Seed Permissions
	if err := database.SeedPermissions(db); err != nil {
		log.Printf("Failed to seed permissions: %v", err)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		BodyLimit:    50 * 1024 * 1024, // 50MB for file uploads
		ErrorHandler: handlers.ErrorHandler,
		ProxyHeader:  fiber.HeaderXForwardedFor,
	})

	clientIP := func(c *fiber.Ctx) string {
		if forwarded := c.Get("CF-Connecting-IP"); forwarded != "" {
			return forwarded
		}
		if forwarded := c.Get("X-Forwarded-For"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
		if realIP := c.Get("X-Real-IP"); realIP != "" {
			return realIP
		}
		return c.IP()
	}

	// Global middleware
	app.Use(logger.New())
	// Security headers
	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if cfg.Environment == "production" {
			c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		return c.Next()
	})
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
		AllowCredentials: true,
	}))
	app.Use(limiter.New(limiter.Config{
		Max:          100, // 100 requests per minute per client IP
		KeyGenerator: clientIP,
	}))

	// Setup routes
	api := app.Group("/api")

	// Auth routes (public)
	auth := api.Group("/auth")
	authHandler := handlers.NewAuthHandler(db, cfg)
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.Refresh)
	auth.Post("/logout", middleware.Auth(cfg), authHandler.Logout)
	auth.Get("/me", middleware.Auth(cfg), authHandler.Me)

	// Public content routes
	publicHandler := handlers.NewPublicHandler(db)
	public := api.Group("/public")
	public.Get("/home", publicHandler.GetHome)
	public.Get("/profile", publicHandler.GetProfile)
	public.Get("/prestasi", publicHandler.GetPrestasi)
	public.Get("/gallery/photo", publicHandler.GetGalleryPhoto)
	public.Get("/gallery/video", publicHandler.GetGalleryVideo)
	public.Get("/articles", publicHandler.GetArticles)
	public.Get("/articles/:slug", publicHandler.GetArticleDetail)
	public.Get("/articles/:slug/comments", publicHandler.GetArticleComments)
	public.Post("/articles/:slug/comments", publicHandler.CreateArticleComment)
	public.Post("/comments/:id/reply", publicHandler.ReplyArticleComment)
	public.Post("/comments/:id/like", publicHandler.LikeComment)
	arrivalHandler := handlers.NewArrivalHandler(db)
	public.Get("/arrivals/:token/search", arrivalHandler.SearchPublic)
	public.Post("/arrivals/:token/submit", arrivalHandler.SubmitPublic)

	// Protected routes
	protected := api.Group("/", middleware.InjectDB(db), middleware.Auth(cfg), middleware.ActivityLog())

	// Profile (self service)
	profile := protected.Group("/profile")
	profile.Get("/", authHandler.Me)
	profile.Post("/avatar", authHandler.UploadProfileAvatar)
	profile.Put("/", authHandler.UpdateProfile)
	profile.Delete("/", authHandler.DeleteProfile)

	// Students
	studentHandler := handlers.NewStudentHandler(db)
	students := protected.Group("/students")
	students.Get("/", middleware.Permission("students.view"), studentHandler.List)
	students.Post("/", middleware.Permission("students.create"), studentHandler.Create)
	students.Post("/import", middleware.Permission("students.create"), studentHandler.ImportCSV)
	students.Get("/template", middleware.Permission("students.view"), studentHandler.ExportTemplate)
	students.Get("/export/excel", middleware.Permission("students.view"), studentHandler.ExportCSV)
	students.Post("/mass-delete", middleware.Permission("students.delete"), studentHandler.MassDelete)
	students.Get("/:id", middleware.Permission("students.view"), studentHandler.Get)
	students.Put("/:id", middleware.Permission("students.edit"), studentHandler.Update)
	students.Delete("/:id", middleware.Permission("students.delete"), studentHandler.Delete)

	// Users (Teachers)
	userHandler := handlers.NewUserHandler(db)
	users := protected.Group("/users")
	users.Get("/", middleware.Permission("users.view"), userHandler.List)
	users.Post("/", middleware.Permission("users.create"), userHandler.Create)
	users.Get("/:id", middleware.Permission("users.view"), userHandler.Get)
	users.Put("/:id", middleware.Permission("users.edit"), userHandler.Update)
	users.Delete("/:id", middleware.Permission("users.delete"), userHandler.Delete)

	// Roles
	roleHandler := handlers.NewRoleHandler(db)
	roles := protected.Group("/roles")
	roles.Get("/", middleware.Permission("roles.view"), roleHandler.List)
	roles.Post("/", middleware.Permission("roles.create"), roleHandler.Create)
	roles.Get("/:id", middleware.Permission("roles.view"), roleHandler.Get)
	roles.Put("/:id", middleware.Permission("roles.edit"), roleHandler.Update)
	roles.Delete("/:id", middleware.Permission("roles.delete"), roleHandler.Delete)

	// Activity logs
	activityLogHandler := handlers.NewActivityLogHandler(db)
	protected.Get("/activity-logs", middleware.Permission("users.view"), activityLogHandler.List)

	// Feature flags
	featureFlagHandler := handlers.NewFeatureFlagHandler(db)
	protected.Get("/feature-flags/ramadhan", featureFlagHandler.GetRamadhan)
	protected.Put("/feature-flags/ramadhan", middleware.Permission("dashboard.view"), featureFlagHandler.SetRamadhan)
	sidebarMenuSettingHandler := handlers.NewSidebarMenuSettingHandler(db)
	protected.Get("/sidebar-menu-settings", sidebarMenuSettingHandler.List)
	protected.Put("/sidebar-menu-settings", sidebarMenuSettingHandler.Update)

	// Permissions
	permHandler := handlers.NewPermissionHandler(db)
	perms := protected.Group("/permissions")
	perms.Post("/", middleware.Permission("permissions.create"), permHandler.Create)

	// Kamar (Rooms)
	kamarHandler := handlers.NewKamarHandler(db)
	kamars := protected.Group("/kamar")
	kamars.Get("/", middleware.Permission("kamar.view"), kamarHandler.List)
	kamars.Post("/", middleware.Permission("kamar.create"), kamarHandler.Create)
	kamars.Get("/:id", middleware.Permission("kamar.view"), kamarHandler.Get)
	kamars.Put("/:id", middleware.Permission("kamar.edit"), kamarHandler.Update)
	kamars.Delete("/:id", middleware.Permission("kamar.delete"), kamarHandler.Delete)
	kamars.Post("/:id/students", middleware.Permission("kamar.edit"), kamarHandler.AddStudent)
	kamars.Delete("/:id/students/:student_id", middleware.Permission("kamar.edit"), kamarHandler.RemoveStudent)

	// Kelas (Classes)
	kelasHandler := handlers.NewKelasHandler(db)
	kelas := protected.Group("/kelas")
	kelas.Get("/stats", middleware.Permission("kelas.view"), kelasHandler.GetStats)
	kelas.Get("/", middleware.Permission("kelas.view"), kelasHandler.List)
	kelas.Post("/", middleware.Permission("kelas.create"), kelasHandler.Create)
	kelas.Get("/:id", middleware.Permission("kelas.view"), kelasHandler.Get)
	kelas.Get("/:id/export-excel", middleware.Permission("kelas.view"), kelasHandler.ExportExcel)
	kelas.Get("/:id/export-pdf", middleware.Permission("kelas.view"), kelasHandler.ExportPDF)
	kelas.Put("/:id", middleware.Permission("kelas.edit"), kelasHandler.Update)
	kelas.Delete("/:id", middleware.Permission("kelas.delete"), kelasHandler.Delete)
	kelas.Post("/:id/students", middleware.Permission("kelas.edit"), kelasHandler.AddStudent)
	kelas.Delete("/:id/students/:student_id", middleware.Permission("kelas.edit"), kelasHandler.RemoveStudent)

	// Lesson Teachers (Guru Mapel)
	ltHandler := handlers.NewLessonTeacherHandler(db)
	kelas.Get("/:id/teachers", middleware.Permission("kelas.view"), ltHandler.ListByClass)
	kelas.Post(":id/teachers", middleware.PermissionAny("kelas.edit", "curriculum.edit", "jadwal_formal.edit", "jadwal_diniyyah.edit"), ltHandler.Assign)
	kelas.Delete(":id/teachers/:assignment_id", middleware.PermissionAny("kelas.edit", "curriculum.edit", "jadwal_formal.edit", "jadwal_diniyyah.edit"), ltHandler.Unassign)

	// Lessons (Mata Pelajaran)
	lessonHandler := handlers.NewLessonHandler(db)
	lessons := protected.Group("/lessons")
	lessons.Get("/", lessonHandler.List) // List all with params
	lessons.Post("/", middleware.Permission("curriculum.create"), lessonHandler.Create)
	lessons.Get("/:id", lessonHandler.Get)
	lessons.Put("/:id", middleware.Permission("curriculum.edit"), lessonHandler.Update)
	lessons.Delete("/:id", middleware.Permission("curriculum.delete"), lessonHandler.Delete)

	// Lesson Assignments (Guru Mapel per Lesson)
	// Reusing ltHandler or creating new
	ltHandlerLesson := handlers.NewLessonTeacherHandler(db)
	lessons.Get("/:id/assignments", middleware.Permission("lessons.view"), ltHandlerLesson.ListByLesson)
	lessons.Post(":id/assignments", middleware.PermissionAny("curriculum.edit", "kelas.edit", "jadwal_formal.edit", "jadwal_diniyyah.edit"), ltHandlerLesson.AssignToLesson)
	lessons.Delete("/assignments/:assignment_id", middleware.PermissionAny("curriculum.edit", "kelas.edit", "jadwal_formal.edit", "jadwal_diniyyah.edit"), ltHandlerLesson.Unassign)

	// Dashboard
	dashHandler := handlers.NewDashboardHandler(db)
	protected.Get("/dashboard/stats", middleware.Permission("dashboard.view"), dashHandler.Stats)
	protected.Get("/dashboard/visitor-stats", middleware.Permission("dashboard.view"), dashHandler.VisitorStats)
	protected.Get("/dashboard/debug-schema", dashHandler.DebugSchema) // Temporary debug route
	protected.Get("/arrivals/dashboard", middleware.Permission("dashboard.view"), arrivalHandler.Dashboard)
	protected.Post("/arrivals/deadline", middleware.Permission("dashboard.view"), arrivalHandler.UpdateStudentDeadline)
	protected.Post("/arrivals/reset-students", middleware.Permission("dashboard.view"), arrivalHandler.ResetStudentArrivals)

	// Notifications
	notifHandler := handlers.NewNotificationHandler(db)
	protected.Get("/notifications", notifHandler.List)
	protected.Put("/notifications/:id/read", notifHandler.MarkRead)
	protected.Get("/notifications/stream", notifHandler.Stream)

	// Schedules
	scheduleHandler := handlers.NewScheduleHandler(db)
	schedules := protected.Group("/schedules")
	// List (General Auth, frontend handles type logic)
	schedules.Get("/", scheduleHandler.List)
	// Create/Edit/Delete (Assuming 'jadwal_formal' permissions apply broadly or admin has them)
	// Ideally should be split, but for now:
	schedules.Post("/", middleware.Permission("jadwal_formal.create"), scheduleHandler.Create)
	schedules.Put("/:id", middleware.Permission("jadwal_formal.edit"), scheduleHandler.Update)
	schedules.Delete("/:id", middleware.Permission("jadwal_formal.delete"), scheduleHandler.Delete)

	// Attendance (Absensi)
	absensiHandler := handlers.NewAbsensiHandler(db)
	attendance := protected.Group("/attendance")
	attendance.Get("/statistics", absensiHandler.GetStatistics)                                                   // Statistics
	attendance.Get("/teacher-statistics", absensiHandler.GetTeacherStatistics)                                    // Teacher Statistics
	attendance.Get("/history", absensiHandler.GetHistory)                                                         // Paginated history
	attendance.Get("/export/pdf", absensiHandler.ExportStatisticsPDF)                                             // Export PDF
	attendance.Get("/export/excel", absensiHandler.ExportStatisticsExcel)                                         // Export Excel
	attendance.Get("/export/teacher/pdf", absensiHandler.ExportTeacherStatisticsPDF)                              // Export Teacher PDF
	attendance.Get("/", absensiHandler.GetAttendance)                                                             // Get form/data
	attendance.Post("/", absensiHandler.SubmitAttendance)                                                         // Submit/Update
	attendance.Post("/teacher", absensiHandler.SubmitTeacherAttendance)                                           // Teacher Attendance
	attendance.Post("/substitute", middleware.Permission("schedule.substitute"), absensiHandler.AssignSubstitute) // Assign Substitute

	// Create a test route to trigger notification (temporary)
	protected.Post("/notifications/test", func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)
		return notifHandler.SendNotification(userID, "Test Notification", "This is a real-time test!", "success", "/dashboard")
	})

	// Halaqoh
	halaqohHandler := handlers.NewHalaqohHandler(db)
	halaqohStatsHandler := handlers.NewHalaqohStatsHandler(db)

	// Laundry Handlers
	laundryVendorHandler := handlers.NewLaundryVendorHandler(db)
	laundryAccountHandler := handlers.NewLaundryAccountHandler(db)
	laundryTransactionHandler := handlers.NewLaundryTransactionHandler(db)
	laundryExportHandler := handlers.NewLaundryExportHandler(db)

	halaqoh := protected.Group("/halaqoh")
	// Assignments
	halaqoh.Get("/assignments", halaqohHandler.ListAssignments)
	halaqoh.Post("/assignments", middleware.Permission("halaqoh-assignments.create"), halaqohHandler.CreateAssignment)
	halaqoh.Put("/assignments/:teacher_id", middleware.Permission("halaqoh-assignments.edit"), halaqohHandler.UpdateAssignment)
	halaqoh.Delete("/assignments/:id", middleware.Permission("halaqoh-assignments.delete"), halaqohHandler.DeleteAssignment)
	halaqoh.Delete("/assignments/teacher/:teacher_id", middleware.Permission("halaqoh-assignments.delete"), halaqohHandler.DeleteAssignmentsByTeacher)
	// Substitute
	halaqoh.Post("/assignments/:id/substitute", middleware.Permission("halaqoh-assignments.edit"), halaqohHandler.AssignSubstitute)
	halaqoh.Delete("/assignments/:id/substitute", middleware.Permission("halaqoh-assignments.edit"), halaqohHandler.UnassignSubstitute)
	// Student Attendance
	halaqoh.Get("/attendance/:assignment_id", halaqohHandler.GetAttendance)
	halaqoh.Post("/attendance/session/:session", halaqohHandler.SubmitSessionAttendance)
	// Teacher Attendance
	halaqoh.Get("/teacher-attendance/:assignment_id", halaqohHandler.GetTeacherAttendance)
	halaqoh.Post("/teacher-attendance/:assignment_id/session/:session", halaqohHandler.SubmitTeacherAttendance)
	// History & Statistics
	halaqoh.Get("/history/students", halaqohHandler.StudentHistory)
	halaqoh.Get("/history/teachers", halaqohHandler.TeacherHistory)
	halaqoh.Get("/statistics/students", halaqohStatsHandler.StudentStatistics)
	halaqoh.Get("/statistics/teachers", halaqohStatsHandler.TeacherStatistics)

	// Exports
	halaqoh.Get("/export/student/pdf", halaqohStatsHandler.ExportHalaqohStudentPDF)
	halaqoh.Get("/export/student/excel", halaqohStatsHandler.ExportHalaqohStudentExcel)
	halaqoh.Get("/export/teacher/pdf", halaqohStatsHandler.ExportHalaqohTeacherPDF)
	halaqoh.Get("/export/teacher/excel", halaqohStatsHandler.ExportHalaqohTeacherExcel)

	// Laundry Routes
	laundry := protected.Group("/laundry")
	// Vendors
	laundry.Get("/vendors", laundryVendorHandler.List)
	laundry.Post("/vendors", laundryVendorHandler.Create)
	laundry.Put("/vendors/:id", laundryVendorHandler.Update)
	laundry.Delete("/vendors/:id", laundryVendorHandler.Delete)
	laundry.Get("/vendors/statistics", laundryVendorHandler.Statistics)
	laundry.Get("/vendors/statistics/:id", laundryVendorHandler.Statistics)

	// Accounts
	laundry.Get("/accounts", laundryAccountHandler.List)
	laundry.Get("/accounts/trashed", laundryAccountHandler.ListTrashed)
	laundry.Post("/accounts", laundryAccountHandler.Create)
	laundry.Put("/accounts/:id", laundryAccountHandler.Update)
	laundry.Put("/accounts/:id/restore", laundryAccountHandler.Restore)
	laundry.Delete("/accounts/:id", laundryAccountHandler.Delete) // Soft Delete
	laundry.Delete("/accounts/:id/force", laundryAccountHandler.ForceDelete)
	laundry.Post("/accounts/bulk", laundryAccountHandler.BulkCreate)
	laundry.Put("/accounts/:id/toggle-block", laundryAccountHandler.ToggleBlock)

	// Transactions
	laundry.Get("/transactions", laundryTransactionHandler.List)
	laundry.Get("/transactions/stats", laundryTransactionHandler.GetAccountStats)
	laundry.Post("/transactions", laundryTransactionHandler.Create)
	laundry.Put("/transactions/:id/pickup", laundryTransactionHandler.MarkAsPickedUp)
	laundry.Post("/transactions/bulk-pickup", laundryTransactionHandler.BulkMarkAsPickedUp)
	laundry.Delete("/transactions/:id", laundryTransactionHandler.Delete)

	// Exports
	laundry.Get("/export/vendors/excel", laundryExportHandler.ExportVendorStatisticsExcel)
	laundry.Get("/export/vendors/pdf", laundryExportHandler.ExportVendorStatisticsPDF)
	laundry.Get("/export/vendors/weekly/pdf", laundryExportHandler.ExportWeeklyVendorStatisticsPDF)
	laundry.Get("/export/vendors/all-weekly/pdf", laundryExportHandler.ExportAllWeeklyVendorStatisticsPDF)
	laundry.Get("/export/accounts/all/pdf", laundryExportHandler.ExportAllAccountsPDF)
	laundry.Get("/export/accounts/exceeded/excel", laundryExportHandler.ExportExceededAccountsExcel)

	// Absensi Ekstra
	ekstraGroupHandler := handlers.NewAbsensiEkstraGroupHandler(db)
	ekstraSessionHandler := handlers.NewAbsensiEkstraSessionHandler(db)

	ekstra := protected.Group("/absensi-ekstra")
	// Groups
	ekstra.Get("/groups", ekstraGroupHandler.List)
	ekstra.Post("/groups", ekstraGroupHandler.Create)
	ekstra.Get("/groups/:id", ekstraGroupHandler.Get)
	ekstra.Put("/groups/:id", ekstraGroupHandler.Update)
	ekstra.Delete("/groups/:id", ekstraGroupHandler.Delete)
	// Group Students
	ekstra.Get("/groups/:id/students", ekstraGroupHandler.GetStudents)
	ekstra.Post("/groups/:id/students", ekstraGroupHandler.AddStudents)
	ekstra.Delete("/groups/:id/students/:student_id", ekstraGroupHandler.RemoveStudent)
	// Group Supervisors
	ekstra.Get("/groups/:id/supervisors", ekstraGroupHandler.GetSupervisors)
	ekstra.Post("/groups/:id/supervisors", ekstraGroupHandler.AddSupervisor)
	ekstra.Delete("/groups/:id/supervisors/:user_id", ekstraGroupHandler.RemoveSupervisor)
	// Sessions
	ekstra.Get("/groups/:group_id/sessions", ekstraSessionHandler.List)
	ekstra.Post("/groups/:group_id/sessions", ekstraSessionHandler.Create)
	ekstra.Get("/sessions/:session_id", ekstraSessionHandler.Get)
	ekstra.Post("/sessions/:session_id/attendance", ekstraSessionHandler.SubmitAttendance)
	ekstra.Post("/sessions/:session_id/close", ekstraSessionHandler.Close)
	ekstra.Delete("/sessions/:session_id", ekstraSessionHandler.Delete)
	// Statistics & Export
	ekstra.Get("/groups/:group_id/statistics", ekstraSessionHandler.Statistics)
	ekstra.Get("/groups/:group_id/export/pdf", ekstraSessionHandler.ExportPDF)

	// Gallery & Albums
	uploadPath := "./uploads"
	if os.Getenv("UPLOAD_PATH") != "" {
		uploadPath = os.Getenv("UPLOAD_PATH")
	}
	galleryHandler := handlers.NewGalleryHandler(db, uploadPath)
	articleHandler := handlers.NewArticleHandler(db, uploadPath)

	gallery := protected.Group("/gallery")
	gallery.Get("/albums", galleryHandler.ListAlbums)
	gallery.Get("/albums/:id", galleryHandler.GetAlbum)
	gallery.Post("/albums", galleryHandler.CreateAlbum)
	gallery.Put("/albums/:id", galleryHandler.UpdateAlbum)
	gallery.Delete("/albums/:id", galleryHandler.DeleteAlbum)
	gallery.Post("/albums/:id/galleries", galleryHandler.AddGalleriesToAlbum)
	gallery.Delete("/albums/:id/galleries/:gallery_id", galleryHandler.RemoveGalleryFromAlbum)
	gallery.Get("/items", galleryHandler.ListGalleries)
	gallery.Post("/items", galleryHandler.UploadGallery)
	gallery.Put("/items/:id", galleryHandler.UpdateGallery)
	gallery.Delete("/items/:id", galleryHandler.DeleteGallery)
	gallery.Post("/items/mass-delete", galleryHandler.MassDeleteGalleries)

	// Articles, Categories & Tags
	articles := protected.Group("/articles")
	articles.Get("/", articleHandler.ListArticles)
	articles.Get("/categories", articleHandler.ListCategories)
	articles.Post("/categories", articleHandler.CreateCategory)
	articles.Put("/categories/:id", articleHandler.UpdateCategory)
	articles.Delete("/categories/:id", articleHandler.DeleteCategory)
	articles.Get("/tags", articleHandler.ListTags)
	articles.Post("/tags", articleHandler.CreateTag)
	articles.Delete("/tags/:id", articleHandler.DeleteTag)
	articles.Get("/:slug", articleHandler.GetArticle)
	articles.Post("/", articleHandler.CreateArticle)
	articles.Put("/:id", articleHandler.UpdateArticle)
	articles.Delete("/:id", articleHandler.DeleteArticle)
	articles.Get("/:id/analytics", articleHandler.GetAnalytics)

	// Prestasi
	prestasiHandler := handlers.NewPrestasiHandler(db, uploadPath)
	prestasi := protected.Group("/prestasi")
	prestasi.Get("/", middleware.Permission("prestasi.view"), prestasiHandler.ListPrestasi)
	prestasi.Post("/", middleware.Permission("prestasi.create"), prestasiHandler.CreatePrestasi)
	prestasi.Put("/:id", middleware.Permission("prestasi.edit"), prestasiHandler.UpdatePrestasi)
	prestasi.Delete("/:id", middleware.Permission("prestasi.delete"), prestasiHandler.DeletePrestasi)

	// Serve uploaded files
	app.Static("/uploads", uploadPath)

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Start server
	port := cfg.Port
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Server starting on :%s", port)
	log.Fatal(app.Listen(":" + port))
}
