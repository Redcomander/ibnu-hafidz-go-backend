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
	command := flag.String("command", "status", "status (read-only) or baseline (metadata only)")
	confirmedDB := flag.String("confirm-database", "", "exact DB_DATABASE name required to register a baseline")
	backupVerified := flag.Bool("backup-verified", false, "confirm a database and upload backup has been restored successfully in isolation")
	flag.Parse()
	if *command != "status" && *command != "baseline" {
		log.Fatal("command must be status or baseline")
	}
	_ = godotenv.Load()
	cfg := config.Load()
	if *command == "baseline" && (!*backupVerified || *confirmedDB != cfg.DBName) {
		log.Fatal("baseline requires -backup-verified and -confirm-database matching DB_DATABASE; see DATABASE_MIGRATIONS.md")
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
	} else {
		err = database.CheckMigrations(db)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Migration baseline 001 verified. Existing application rows and schema were not modified by this command.")
}
