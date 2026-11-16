package handlers

import (
	"fmt"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"time"
)

type laundryInputUser struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}
type laundryTransactionView struct {
	models.LaundryTransaction
	InputBy      *laundryInputUser `json:"input_by"`
	SameDayCount int64             `json:"same_day_count"`
}

func laundryOwnerKey(student, user, account uint, date string) string {
	if student > 0 {
		return fmt.Sprintf("student:%d:%s", student, date)
	}
	if user > 0 {
		return fmt.Sprintf("user:%d:%s", user, date)
	}
	return fmt.Sprintf("account:%d:%s", account, date)
}

// Counts related transactions in one bounded query, independent of search,
// vendor/status filters and pagination. A similarity marker is not a verdict.
func (h *LaundryTransactionHandler) annotateTransactions(items []models.LaundryTransaction) ([]laundryTransactionView, error) {
	result := make([]laundryTransactionView, 0, len(items))
	if len(items) == 0 {
		return result, nil
	}
	ids, accounts, students, users := []uint{}, []uint{}, []uint{}, []uint{}
	minDate, maxDate := items[0].Tanggal, items[0].Tanggal
	for _, item := range items {
		ids = append(ids, item.ID)
		accounts = append(accounts, item.LaundryAccountID)
		if item.Tanggal.Before(minDate) {
			minDate = item.Tanggal
		}
		if item.Tanggal.After(maxDate) {
			maxDate = item.Tanggal
		}
		if item.Account != nil {
			if item.Account.StudentID != nil {
				students = append(students, *item.Account.StudentID)
			}
			if item.Account.UserID != nil {
				users = append(users, *item.Account.UserID)
			}
		}
	}
	type group struct {
		StudentID uint
		UserID    uint
		AccountID uint
		Day       string
		Total     int64
	}
	var groups []group
	ownerUser := "CASE WHEN a.student_id IS NULL THEN a.user_id ELSE NULL END"
	ownerAccount := "CASE WHEN a.student_id IS NULL AND a.user_id IS NULL THEN t.laundry_account_id ELSE 0 END"
	err := h.db.Table("laundry_transactions AS t").
		Joins("LEFT JOIN laundry_accounts AS a ON a.id = t.laundry_account_id AND a.deleted_at IS NULL").
		Select("a.student_id AS student_id, "+ownerUser+" AS user_id, "+ownerAccount+" AS account_id, DATE(t.tanggal) AS day, COUNT(*) AS total").
		Where("t.deleted_at IS NULL AND t.tanggal >= ? AND t.tanggal < ?", minDate.Format("2006-01-02"), maxDate.AddDate(0, 0, 1).Format("2006-01-02")).
		Where("t.laundry_account_id IN ? OR a.student_id IN ? OR a.user_id IN ?", accounts, students, users).
		Group("a.student_id, " + ownerUser + ", " + ownerAccount + ", DATE(t.tanggal)").Scan(&groups).Error
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	for _, g := range groups {
		counts[laundryOwnerKey(g.StudentID, g.UserID, g.AccountID, g.Day)] = g.Total
	}
	inputs := map[uint]*laundryInputUser{}
	// Old deployments can still read transactions prior to the additive migration.
	if h.db.Migrator().HasTable("laundry_transaction_inputs") {
		var rows []models.LaundryTransactionInput
		if err := h.db.Where("transaction_id IN ?", ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			inputs[row.TransactionID] = &laundryInputUser{ID: row.ActorID, Name: row.ActorName}
		}
	}
	for _, item := range items {
		var student, user uint
		if item.Account != nil {
			if item.Account.StudentID != nil {
				student = *item.Account.StudentID
			}
			if item.Account.UserID != nil {
				user = *item.Account.UserID
			}
		}
		count := counts[laundryOwnerKey(student, user, item.LaundryAccountID, item.Tanggal.Format(time.DateOnly))]
		if count < 1 {
			count = 1
		}
		result = append(result, laundryTransactionView{LaundryTransaction: item, InputBy: inputs[item.ID], SameDayCount: count})
	}
	return result, nil
}
