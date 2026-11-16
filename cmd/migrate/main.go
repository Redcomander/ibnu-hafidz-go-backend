package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/database"
	"github.com/joho/godotenv"
)

func main() {
	command := flag.String("command", "status", "status, baseline, mobile-releases, or laundry-inputs (additive tables)")
	confirmedDB := flag.String("confirm-database", "", "exact DB_DATABASE name required to register a baseline")
	backupVerified := flag.Bool("backup-verified", false, "confirm a database and upload backup has been restored successfully in isolation")
	unrestoredBackup := flag.String("backup-without-restore", "", "laundry-inputs only: gzip backup path; explicitly proceed without restore verification")
	flag.Parse()
	if *command != "status" && *command != "baseline" && *command != "mobile-releases" && *command != "laundry-inputs" {
		log.Fatal("command must be status, baseline, mobile-releases, or laundry-inputs")
	}
	_ = godotenv.Load()
	cfg := config.Load()
	if err := validateBackup(*command, *confirmedDB, cfg.DBName, *backupVerified, *unrestoredBackup); err != nil {
		log.Fatal(err)
	}
	if *unrestoredBackup != "" {
		log.Print("Backup gzip integrity checked. Restore NOT verified; proceeding by explicit operator choice for laundry-inputs only.")
	}
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()
	if *command == "baseline" {
		err = database.RegisterBaseline(db)
	} else if *command == "mobile-releases" {
		err = database.ApplyMobileReleases(db)
	} else if *command == "laundry-inputs" {
		err = database.ApplyLaundryInputs(db)
	} else {
		err = database.CheckMigrations(db)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Migration command completed. No existing business records were changed.")
}
