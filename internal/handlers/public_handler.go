package handlers

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type PublicTextItem struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Image       string   `json:"image,omitempty"`
	Points      []string `json:"points,omitempty"`
}

type PublicTestimonial struct {
	Name     string `json:"name"`
	Relation string `json:"relation"`
	Quote    string `json:"quote"`
}

type PublicContact struct {
	Address       string `json:"address"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	VisitingHours string `json:"visiting_hours"`
	WhatsAppLink  string `json:"whatsapp_link"`
}

type PublicScheduleItem struct {
	Time      string `json:"time"`
	Activity  string `json:"activity"`
	Note      string `json:"note"`
	Highlight bool   `json:"highlight,omitempty"`
}

type PublicTeamMember struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Description string `json:"description"`
}

type PublicProfileContent struct {
	HistoryParagraphs []string             `json:"history_paragraphs"`
	Vision            string               `json:"vision"`
	Missions          []string             `json:"missions"`
	Programs          []PublicTextItem     `json:"programs"`
	DailySchedule     []PublicScheduleItem `json:"daily_schedule"`
	TeamMembers       []PublicTeamMember   `json:"team_members"`
	CTA               PublicTextItem       `json:"cta"`
}

type PublicHomeContent struct {
	Programs        []PublicTextItem    `json:"programs"`
	DailyActivity   PublicTextItem      `json:"daily_activity"`
	Facilities      []PublicTextItem    `json:"facilities"`
	Extracurricular []PublicTextItem    `json:"extracurricular"`
	Testimonials    []PublicTestimonial `json:"testimonials"`
	Contact         PublicContact       `json:"contact"`
	CTA             PublicTextItem      `json:"cta"`
}

type PublicHandler struct {
	db *gorm.DB
}

type publicCommentPayload struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Body  string `json:"body"`
}

func NewPublicHandler(db *gorm.DB) *PublicHandler {
	return &PublicHandler{db: db}
}

func (h *PublicHandler) GetHome(c *fiber.Ctx) error {
	var studentCount int64
	var teacherCount int64

	h.db.Model(&models.Student{}).Count(&studentCount)
	h.db.Model(&models.User{}).Count(&teacherCount)

	var articles []models.Article
	h.db.
		Preload("Category").
		Preload("Author").
		Where("status = ?", "published").
		Where("published_at IS NOT NULL").
		Order("published_at DESC").
		Limit(3).
		Find(&articles)

	var heroImages []models.Gallery
	h.db.
		Where("type = ?", "photo").
		Order("created_at DESC").
		Limit(5).
		Find(&heroImages)

	currentYear := time.Now().Year()
	yearsEstablished := currentYear - 2018
	if yearsEstablished < 0 {
		yearsEstablished = 0
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"stats": fiber.Map{
				"santri_aktif":         studentCount,
				"ustadz_berpengalaman": teacherCount,
				"tahun_berdiri":        yearsEstablished,
			},
			"articles":    articles,
			"hero_images": heroImages,
			"content":     buildPublicHomeContent(),
		},
	})
}

func (h *PublicHandler) GetProfile(c *fiber.Ctx) error {
	var studentCount int64
	var teacherCount int64

	h.db.Model(&models.Student{}).Count(&studentCount)
	h.db.Model(&models.User{}).Count(&teacherCount)

	currentYear := time.Now().Year()
	yearsEstablished := currentYear - 2018
	if yearsEstablished < 0 {
		yearsEstablished = 0
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"stats": fiber.Map{
				"santri_aktif":    studentCount,
				"tenaga_pengajar": teacherCount,
				"tahun_berdiri":   yearsEstablished,
			},
			"content": buildPublicProfileContent(),
		},
	})
}

func (h *PublicHandler) GetPrestasi(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	perPage := c.QueryInt("per_page", 12)
	if perPage < 1 {
		perPage = 12
	}

	search := c.Query("search")
	kategori := c.Query("kategori")
	tingkat := c.Query("tingkat")

	query := h.db.Model(&models.Prestasi{}).Preload("Student")
	if search != "" {
		query = query.Where("nama_prestasi LIKE ? OR deskripsi LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if kategori != "" {
		query = query.Where("kategori = ?", kategori)
	}
	if tingkat != "" {
		query = query.Where("tingkat = ?", tingkat)
	}

	var total int64
	query.Count(&total)

	var items []models.Prestasi
	offset := (page - 1) * perPage
	query.Order("created_at DESC").Limit(perPage).Offset(offset).Find(&items)

	var categories []string
	h.db.Model(&models.Prestasi{}).Distinct("kategori").Where("kategori <> ''").Order("kategori ASC").Pluck("kategori", &categories)

	var levels []string
	h.db.Model(&models.Prestasi{}).Distinct("tingkat").Where("tingkat IS NOT NULL AND tingkat <> ''").Order("tingkat ASC").Pluck("tingkat", &levels)

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"items":       items,
			"total":       total,
			"page":        page,
			"per_page":    perPage,
			"total_pages": (total + int64(perPage) - 1) / int64(perPage),
			"categories":  categories,
			"levels":      levels,
		},
	})
}

func (h *PublicHandler) GetGalleryPhoto(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	perPage := c.QueryInt("per_page", 20)
	if perPage < 1 {
		perPage = 20
	}
	search := c.Query("search")
	album := c.Query("album")

	query := h.db.Model(&models.Gallery{}).
		Preload("Album").
		Where("type = ?", "photo")

	if search != "" {
		query = query.Where("title LIKE ?", "%"+search+"%")
	}
	if album != "" {
		query = query.Joins("LEFT JOIN albums ON albums.id = galleries.album_id").Where("albums.slug = ?", album)
	}

	var total int64
	query.Count(&total)

	var items []models.Gallery
	offset := (page - 1) * perPage
	query.Order("galleries.created_at DESC").Limit(perPage).Offset(offset).Find(&items)

	var albums []models.Album
	h.db.Model(&models.Album{}).Order("title ASC").Find(&albums)

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"items":       items,
			"total":       total,
			"page":        page,
			"per_page":    perPage,
			"total_pages": (total + int64(perPage) - 1) / int64(perPage),
			"albums":      albums,
		},
	})
}

func (h *PublicHandler) GetGalleryVideo(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	perPage := c.QueryInt("per_page", 20)
	if perPage < 1 {
		perPage = 20
	}
	search := c.Query("search")
	source := c.Query("source")
	album := c.Query("album")

	query := h.db.Model(&models.Gallery{}).
		Preload("Album").
		Where("type = ?", "video")

	if search != "" {
		query = query.Where("title LIKE ?", "%"+search+"%")
	}
	if source != "" && source != "all" {
		query = query.Where("source = ?", source)
	}
	if album != "" {
		query = query.Joins("LEFT JOIN albums ON albums.id = galleries.album_id").Where("albums.slug = ?", album)
	}

	var total int64
	query.Count(&total)

	var items []models.Gallery
	offset := (page - 1) * perPage
	query.Order("galleries.created_at DESC").Limit(perPage).Offset(offset).Find(&items)

	for i := range items {
		items[i].Path = normalizePublicMediaPath(items[i])
	}

	var albums []models.Album
	h.db.Model(&models.Album{}).Order("title ASC").Find(&albums)

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"items":       items,
			"total":       total,
			"page":        page,
			"per_page":    perPage,
			"total_pages": (total + int64(perPage) - 1) / int64(perPage),
			"albums":      albums,
		},
	})
}

func normalizePublicMediaPath(item models.Gallery) string {
	if item.Source == "upload" {
		if strings.HasPrefix(item.Path, "http") || strings.HasPrefix(item.Path, "/") {
			return item.Path
		}
		return "/uploads/" + item.Path
	}

	if item.Source == "url" && strings.HasPrefix(item.Path, "url:") {
		return strings.TrimPrefix(item.Path, "url:")
	}

	if strings.Contains(item.Path, ":") {
		parts := strings.SplitN(item.Path, ":", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}

	if item.OriginalURL != nil && *item.OriginalURL != "" {
		return *item.OriginalURL
	}

	return item.Path
}

func buildPublicHomeContent() PublicHomeContent {
	return PublicHomeContent{
		Programs: []PublicTextItem{
			{Title: "Program Tahfidz Sangat Intensif", Description: "1 Guru tahfidz untuk 12 santri. Kegiatan menghafal Al Quran 3 kali sehari, ba'da subuh, ba'da ashar dan ba'da isya."},
			{Title: "Kajian Kitab Kuning", Description: "Seluruh santri wajib bisa membaca dan belajar kitab kuning. Metode yang di gunakan adalah metode Amsilati, yang praktis dan sudah teruji."},
			{Title: "Lancar Bahasa Arab dan Inggris", Description: "Santri di biasakan untuk pede dan lancar bahasa Arab dan Inggris, baik berbicara maupun untuk pidato/ceramah."},
			{Title: "Bimbingan Kuliah ke Luar Negeri", Description: "Bagi santri yang berminat kuliah ke luar negeri ada bimbingan khusus kuliah ke luar negeri oleh alumni luar negeri."},
		},
		DailyActivity: PublicTextItem{
			Title:       "Jadwal Kegiatan Harian Santri",
			Description: "Pondok Pesantren Ibnu Hafidz",
			Image:       "/jadwal.jpeg",
		},
		Facilities: []PublicTextItem{
			{Title: "Masjid dan Ruang Ibadah", Description: "Pusat aktivitas ibadah berjamaah, kultum, tahsin, dan pembinaan ruhiyah santri.", Image: "/welcome2.JPG"},
			{Title: "Ruang Tahfidz", Description: "Ruang belajar untuk halaqah hafalan yang nyaman dan kondusif sepanjang hari.", Image: "/welcome5.jpeg"},
			{Title: "Asrama Santri", Description: "Area hunian santri yang mendukung pembiasaan disiplin, kemandirian, dan kebersamaan.", Image: "/welcome2.JPG"},
			{Title: "Area Olahraga dan Ekskul", Description: "Fasilitas kegiatan fisik dan pengembangan bakat untuk pembinaan santri secara utuh.", Image: "/welcome5.jpeg"},
		},
		Extracurricular: []PublicTextItem{
			{Title: "Muhadharah (Public Speaking)", Description: "Latihan pidato dalam tiga bahasa (Indonesia, Arab, Inggris) untuk mempersiapkan santri menjadi da'i masa depan"},
			{Title: "Pencak Silat", Description: "Seni bela diri tradisional untuk melatih kedisiplinan, ketangkasan, dan ketahanan fisik santri"},
			{Title: "Karate", Description: "Bela diri dari Jepang yang mengajarkan disiplin, fokus, dan teknik pertahanan diri"},
			{Title: "Boxing", Description: "Olahraga tinju untuk melatih kekuatan, ketahanan, dan strategi dalam pertandingan"},
			{Title: "Tricking", Description: "Kombinasi gerakan akrobatik seperti parkour yang mengembangkan kelenturan dan keberanian santri"},
			{Title: "Bulu Tangkis", Description: "Olahraga raket yang melatih kelincahan, refleks, dan koordinasi mata-tangan"},
			{Title: "Hadroh", Description: "Seni musik islami dengan rebana yang mengembangkan keterampilan musikal dan melestarikan tradisi"},
			{Title: "Qori'", Description: "Pelatihan seni baca Al-Qur'an dengan tajwid dan irama yang indah"},
			{Title: "Futsal", Description: "Olahraga tim yang mengembangkan kerja sama, strategi, dan kebugaran fisik"},
			{Title: "Voli", Description: "Olahraga bola voli yang melatih koordinasi, kerjasama tim, dan kemampuan motorik"},
			{Title: "Jurnalistik & Media", Description: "Pelatihan penulisan, fotografi, dan produksi konten digital untuk mengembangkan keterampilan komunikasi"},
		},
		Testimonials: []PublicTestimonial{
			{Name: "Testimoni Tamu Ibnu Hafidz", Relation: "Kunjungan Ketua PWNU Jabar", Quote: "Testimoni Kunjungan Ketua PWNU Jabar"},
			{Name: "Testimoni Orang Tua", Relation: "Kepuasan wali santri", Quote: "Kepuasan wali santri"},
			{Name: "Testimoni Orang Tua", Relation: "Kepuasan wali santri", Quote: "Kepuasan wali santri"},
		},
		Contact: PublicContact{
			Address:       "Sukamulya, RT.25/RW.06, Rancadaka, Kec. Pusakanagara, Kabupaten Subang, Jawa Barat 41255",
			Phone:         "0812-1227-2775",
			Email:         "yayasanibnuhafidzcenter@ibnuhafidz.ponpes.id",
			VisitingHours: "Senin - Minggu (24 Jam)",
			WhatsAppLink:  "https://wa.me/6281212272775?text=Assalamu'alaikum%20Warahmatullahi%20Wabarakatuh.%0ASaya%20ingin%20bertanya%20lebih%20lanjut%20tentang%20Pondok%20Pesantren%20Ibnu%20Hafidz.",
		},
		CTA: PublicTextItem{
			Title:       "Bergabunglah dengan Keluarga Besar Kami",
			Description: "Jadilah bagian dari keluarga besar Pondok Pesantren Ibnu Hafidz dan raih masa depan cerah dengan pendidikan berkualitas yang menggabungkan ilmu agama dan pengetahuan modern.",
		},
	}
}

func buildPublicProfileContent() PublicProfileContent {
	return PublicProfileContent{
		HistoryParagraphs: []string{
			"Pesantren Tahfidz Ibnu Hafidz didirikan oleh K.H Taslim Hafidz, Lc. (Alumni Universitas Al-Azhar Cairo Mesir) pada tahun 2018 dengan luas tanah 46.000 m2. Diresmikan pada tanggal 10 Juli 2019 oleh Gubernur TGB H. Dr. Zainul Majdi, Lc. MA.",
			"Santri perdana tahun ajaran 2019 - 2020 berjumlah 40 santri, dan saat ini sudah mencapai ratusan santri dari berbagai daerah seperti Papua, Sulawesi, Aceh, dan juga dari Malaysia, dan didominasi dari Jawa Barat.",
			"Tenaga pengajar terdiri dari lulusan Universitas Al-Azhar Cairo Mesir, Pesantren Gontor, Pesantren Almawaddah, Pesantren Sidogiri, Alfalah Ploso, Dalwa, Ponpes Al Ashriyyah Nurul Iman, UI, UGM, UNDIP, UNM, dan Pesantren Daarul Qur'an milik Ust. Yusuf Mansur.",
		},
		Vision: "Menjadi Pesantren Terbaik Di Indonesia. Melahirkan Generasi Berakhlak Mulia Yang Mandiri dan Memiliki Jiwa Kepemimpinan, Berwawasan Global dan Berprestasi Tingkat Internasional.",
		Missions: []string{
			"Menyelenggarakan pendidikan yang profesional, berorientasi pada mutu, spiritual, intelektual dan moral, berbasis Al-Qur'an.",
			"Menerapkan pembelajaran aktif, inovatif, kreatif dan efektif.",
			"Membina dan mendidik generasi muslim, memiliki jiwa yang ikhlas, mandiri, sehat jasmani dan rohani, berakhlak mulia serta bermanfaat untuk semua.",
			"Menyelenggarakan program intensif agar peserta didik mampu melanjutkan ke Perguruan Tinggi Negeri dan Luar Negeri.",
			"Mengembangkan keunggulan potensi dan berkompetensi di dunia internasional.",
		},
		Programs: []PublicTextItem{
			{Title: "Tahfidz Halaqoh", Description: "Program intensif menghafal Al-Qur'an dengan metode halaqoh yang dibimbing langsung oleh para hafidz dan hafidzah berpengalaman.", Points: []string{"Metode menghafal yang disesuaikan dengan kemampuan santri", "Bimbingan intensif dengan rasio pengajar dan santri yang ideal", "Evaluasi hafalan berkala untuk memastikan kualitas hafalan", "Pembelajaran tajwid dan tahsin untuk memperbaiki bacaan", "Program muroja'ah (pengulangan) terjadwal untuk menjaga hafalan"}},
			{Title: "Kajian Kitab", Description: "Pembelajaran kitab-kitab klasik dan kontemporer untuk memperdalam pemahaman agama Islam dari berbagai aspek keilmuan.", Points: []string{"Kitab Tafsir (Tafsir Ibnu Katsir, Tafsir Al-Qurthubi)", "Kitab Hadits (Riyadhus Shalihin, Bulughul Maram)", "Kitab Fiqih (Fathul Qarib, Fiqhus Sunnah)", "Kitab Akhlak (Ta'limul Muta'allim, Akhlaq lil Banin/Banat)", "Kitab Bahasa Arab (Nahwu, Sharaf, Balaghah)"}},
			{Title: "Sekolah Formal SMP IT", Description: "Pendidikan formal tingkat menengah pertama dengan kurikulum nasional yang diintegrasikan dengan nilai-nilai Islam.", Points: []string{"Kurikulum Nasional (Kemendikbud) dengan pengayaan materi keislaman", "Program penguatan karakter berbasis nilai-nilai Islam", "Pembelajaran bahasa Arab dan Inggris intensif", "Integrasi tahfidz dalam jadwal akademik", "Pengembangan life skill dan leadership"}},
			{Title: "Sekolah Formal SMA IT", Description: "Pendidikan formal tingkat menengah atas dengan kurikulum nasional yang diintegrasikan dengan nilai-nilai Islam dan persiapan perguruan tinggi.", Points: []string{"Penjurusan IPA dan IPS dengan pendekatan islami", "Persiapan UTBK dan seleksi perguruan tinggi dalam dan luar negeri", "Program beasiswa ke universitas terkemuka", "Pengembangan riset dan karya ilmiah", "Bimbingan karir dan pengembangan bakat"}},
		},
		DailySchedule: []PublicScheduleItem{
			{Time: "04:00 - 04:30", Activity: "Tahajud", Note: "Shalat malam dan murojaah hafalan"},
			{Time: "04:30 - 05:00", Activity: "Sholat Subuh Berjamaah", Note: "Berjamaah di masjid"},
			{Time: "05:00 - 06:15", Activity: "Tahfidz Pertama", Note: "Setoran hafalan baru", Highlight: true},
			{Time: "06:15 - 06:30", Activity: "Sholat Dhuha", Note: "Shalat sunnah pagi"},
			{Time: "06:30 - 07:30", Activity: "Istirahat dan Makan Pagi", Note: "Sarapan dan persiapan sekolah"},
			{Time: "07:30 - 08:00", Activity: "Giving Vocabulary", Note: "Penguatan bahasa Arab dan Inggris"},
			{Time: "08:00 - 11:45", Activity: "Sekolah Formal SMPIT dan SMAIT", Note: "Pembelajaran akademik"},
			{Time: "12:00 - 12:30", Activity: "Sholat Dzuhur Berjamaah", Note: "Berjamaah di masjid"},
			{Time: "12:30 - 13:00", Activity: "Makan Siang", Note: "Istirahat"},
			{Time: "13:00 - 14:00", Activity: "Tidur Siang", Note: "Istirahat santri"},
			{Time: "14:00 - 15:15", Activity: "Belajar Kitab Kuning", Note: "Pembelajaran kitab klasik"},
			{Time: "15:15 - 15:45", Activity: "Sholat Ashar dan Al-Waqiah", Note: "Berjamaah di masjid"},
			{Time: "15:45 - 16:45", Activity: "Tahfidz Kedua", Note: "Murojaah hafalan", Highlight: true},
			{Time: "16:45 - 17:30", Activity: "Olahraga dan Ekskul", Note: "Pengembangan bakat dan fisik"},
			{Time: "17:30 - 18:00", Activity: "Arhamarrohimin dan Kultum", Note: "Kegiatan spiritual"},
			{Time: "18:00 - 18:20", Activity: "Sholat Maghrib Berjamaah", Note: "Berjamaah di masjid"},
			{Time: "18:20 - 18:45", Activity: "Tahsin", Note: "Perbaikan bacaan Al-Quran"},
			{Time: "18:45 - 19:15", Activity: "Makan Malam", Note: "Istirahat"},
			{Time: "19:15 - 19:45", Activity: "Sholat Isya Berjamaah", Note: "Berjamaah di masjid"},
			{Time: "19:45 - 21:00", Activity: "Tahfidz Ketiga", Note: "Persiapan hafalan", Highlight: true},
			{Time: "21:00 - 21:30", Activity: "Al-Mulk dan Tikror Mufrodat", Note: "Spiritual dan bahasa"},
			{Time: "21:30 - 04:00", Activity: "Istirahat Malam", Note: "Tidur malam"},
		},
		TeamMembers: []PublicTeamMember{
			{Name: "K.H Taslim hafidz, Lc", Role: "Pendiri Pondok", Description: "Belajar di Al-Mawaddah Jakarta, lanjut ke Suriah, & Sarjana di Al-Azhar. Di Damaskus, Beliau belajar langsung dengan Syekh Sa'id Ramadhan Al-Bouty (ilmuwan Suriah, salah satu ulama' rujukan dunia)."},
		},
		CTA: PublicTextItem{
			Title:       "Bergabunglah dengan Kami",
			Description: "Jadilah bagian dari keluarga besar Pondok Pesantren Ibnu Hafidz dan raih masa depan cerah dengan pendidikan berkualitas yang mengintegrasikan nilai-nilai Islam.",
		},
	}
}

// GetArticles returns a paginated list of published articles
func (h *PublicHandler) GetArticles(c *fiber.Ctx) error {
	var articles []models.Article
	var total int64

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 12)
	search := c.Query("search", "")

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 12
	}
	if limit > 100 {
		limit = 100
	}

	query := h.db.Model(&models.Article{}).Where("status = ?", "published").Where("published_at IS NOT NULL")

	if search != "" {
		query = query.Where("LOWER(title) LIKE ?", "%"+strings.ToLower(search)+"%")
	}

	query.Count(&total)

	offset := (page - 1) * limit
	query.Preload("Category").Preload("Author").
		Order("published_at DESC").
		Offset(offset).Limit(limit).
		Find(&articles)

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	// Batch-fetch view counts for the current page
	type viewCountRow struct {
		ArticleID uint
		Count     int64
	}
	ids := make([]uint, len(articles))
	for i, a := range articles {
		ids[i] = a.ID
	}
	var vcRows []viewCountRow
	if len(ids) > 0 {
		h.db.Model(&models.ArticleView{}).
			Select("article_id, COUNT(*) as count").
			Where("article_id IN ?", ids).
			Group("article_id").
			Scan(&vcRows)
	}
	vcMap := make(map[uint]int64, len(vcRows))
	for _, r := range vcRows {
		vcMap[r.ArticleID] = r.Count
	}

	type publicArticleItem struct {
		models.Article
		Views int64 `json:"views"`
	}
	result := make([]publicArticleItem, len(articles))
	for i, a := range articles {
		result[i] = publicArticleItem{Article: a, Views: vcMap[a.ID]}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"articles":    result,
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

// publicArticleDetailResponse wraps an article with computed view/reading metrics.
type publicArticleDetailResponse struct {
	models.Article
	Views       int64  `json:"views"`
	UniqueViews int64  `json:"unique_views"`
	ReadingTime string `json:"reading_time"`
}

// GetArticleDetail returns a single published article by slug and records a view.
func (h *PublicHandler) GetArticleDetail(c *fiber.Ctx) error {
	slug := c.Params("slug")
	var article models.Article

	if err := h.db.Preload("Category").Preload("Author").
		Where("slug = ? AND status = ? AND published_at IS NOT NULL", slug, "published").
		First(&article).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Article not found",
		})
	}

	// Record view — deduplicate same IP within 1 hour
	var existingViewCount int64
	h.db.Model(&models.ArticleView{}).
		Where("article_id = ? AND ip_address = ? AND created_at >= ?", article.ID, c.IP(), time.Now().Add(-time.Hour)).
		Count(&existingViewCount)
	if existingViewCount == 0 {
		h.db.Create(&models.ArticleView{
			ArticleID: article.ID,
			IPAddress: c.IP(),
			UserAgent: c.Get("User-Agent"),
		})
	}

	var totalViews, uniqueViews int64
	h.db.Model(&models.ArticleView{}).Where("article_id = ?", article.ID).Count(&totalViews)
	h.db.Model(&models.ArticleView{}).Where("article_id = ?", article.ID).Distinct("ip_address").Count(&uniqueViews)

	wordCount := len(strings.Fields(article.Body))
	readingTime := fmt.Sprintf("%d menit baca", (wordCount/200)+1)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data": publicArticleDetailResponse{
			Article:     article,
			Views:       totalViews,
			UniqueViews: uniqueViews,
			ReadingTime: readingTime,
		},
	})
}

// GetArticleComments returns approved comments (with replies) for an article by slug.
func (h *PublicHandler) GetArticleComments(c *fiber.Ctx) error {
	slug := c.Params("slug")
	var article models.Article

	if err := h.db.Where("slug = ? AND status = ? AND published_at IS NOT NULL", slug, "published").First(&article).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Article not found",
		})
	}

	var comments []models.Comment
	if err := h.db.
		Where("article_id = ? AND parent_id IS NULL AND is_approved = ?", article.ID, true).
		Preload("Replies", "is_approved = ?", true).
		Order("created_at DESC").
		Find(&comments).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Failed to load comments",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    comments,
	})
}

// CreateArticleComment creates a public comment for an article by slug.
func (h *PublicHandler) CreateArticleComment(c *fiber.Ctx) error {
	slug := c.Params("slug")
	var article models.Article

	if err := h.db.Where("slug = ? AND status = ? AND published_at IS NOT NULL", slug, "published").First(&article).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Article not found",
		})
	}

	var payload publicCommentPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid payload",
		})
	}

	payload.Name = strings.TrimSpace(payload.Name)
	payload.Email = strings.TrimSpace(payload.Email)
	payload.Body = strings.TrimSpace(payload.Body)

	if payload.Name == "" || payload.Body == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Name and comment are required",
		})
	}

	comment := models.Comment{
		ArticleID:  article.ID,
		Name:       payload.Name,
		Email:      payload.Email,
		Body:       payload.Body,
		IsApproved: false,
		LikesCount: 0,
	}

	if err := h.db.Create(&comment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Failed to create comment",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    comment,
	})
}

// ReplyArticleComment creates a public reply to an existing approved comment.
func (h *PublicHandler) ReplyArticleComment(c *fiber.Ctx) error {
	commentID := c.Params("id")
	var parent models.Comment

	if err := h.db.Where("id = ? AND is_approved = ?", commentID, true).First(&parent).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Parent comment not found",
		})
	}

	var payload publicCommentPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid payload",
		})
	}

	payload.Name = strings.TrimSpace(payload.Name)
	payload.Email = strings.TrimSpace(payload.Email)
	payload.Body = strings.TrimSpace(payload.Body)

	if payload.Name == "" || payload.Body == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Name and reply are required",
		})
	}

	reply := models.Comment{
		ArticleID:  parent.ArticleID,
		ParentID:   &parent.ID,
		Name:       payload.Name,
		Email:      payload.Email,
		Body:       payload.Body,
		IsApproved: false,
		LikesCount: 0,
	}

	if err := h.db.Create(&reply).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Failed to create reply",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    reply,
	})
}

// LikeComment increments likes_count for a comment.
func (h *PublicHandler) LikeComment(c *fiber.Ctx) error {
	commentID := c.Params("id")

	res := h.db.Model(&models.Comment{}).Where("id = ? AND is_approved = ?", commentID, true).
		UpdateColumn("likes_count", gorm.Expr("COALESCE(likes_count, 0) + 1"))
	if res.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Failed to like comment",
		})
	}
	if res.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Comment not found",
		})
	}

	var comment models.Comment
	if err := h.db.Select("id", "likes_count").Where("id = ?", commentID).First(&comment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Failed to fetch updated likes",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"id":          comment.ID,
			"likes_count": comment.LikesCount,
		},
	})
}
