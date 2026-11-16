package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

func validateBackup(command, confirmedDB, targetDB string, verified bool, unrestored string) error {
	if unrestored != "" && (command != "laundry-inputs" || verified) {
		return fmt.Errorf("-backup-without-restore is exclusive to laundry-inputs and cannot be combined with -backup-verified")
	}
	if command == "status" {
		return nil
	}
	if targetDB == "" || confirmedDB != targetDB {
		return fmt.Errorf("-confirm-database must match DB_DATABASE")
	}
	if verified {
		return nil
	}
	if unrestored == "" {
		return fmt.Errorf("requires -backup-verified; laundry-inputs also accepts -backup-without-restore=/absolute/backup.sql.gz")
	}
	file, err := os.Open(unrestored)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup must be a regular gzip file")
	}
	reader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read gzip backup: %w", err)
	}
	defer reader.Close()
	size, err := io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("backup integrity failed: %w", err)
	}
	if size == 0 {
		return fmt.Errorf("backup is empty")
	}
	return nil
}
