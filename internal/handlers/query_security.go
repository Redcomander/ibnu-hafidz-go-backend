package handlers

import (
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// primaryKey never lets a string ID become a GORM SQL condition. Invalid
// numeric IDs deliberately match no row, including on MySQL's coercive types.
func primaryKey(value interface{}) clause.Expression {
	if raw, ok := value.(string); ok {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			return clause.Expr{SQL: "1 = 0"}
		}
		value = id
	}
	return clause.Eq{Column: clause.PrimaryColumn, Value: value}
}

// Model metadata is the allowlist: SQL expressions and unknown columns cannot
// enter ORDER BY. Structured columns also stay qualified when a list joins.
func safeOrder(db *gorm.DB, field, direction string) *gorm.DB {
	stmt := &gorm.Statement{DB: db}
	if db.Statement.Model == nil || stmt.Parse(db.Statement.Model) != nil {
		return db
	}
	column := stmt.Schema.LookUpField(field)
	if column == nil || column.DBName == "" {
		column = stmt.Schema.LookUpField("created_at")
	}
	if column == nil {
		return db
	}
	return db.Order(clause.OrderByColumn{
		Column: clause.Column{Table: stmt.Schema.Table, Name: column.DBName},
		Desc:   strings.ToLower(direction) != "asc",
	})
}
