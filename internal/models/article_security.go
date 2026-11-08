package models

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"gorm.io/gorm"
)

var articleHTMLPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowStyles("text-align").Matching(regexp.MustCompile(`^(left|right|center|justify)$`)).OnElements("p", "h1", "h2", "h3", "h4", "h5", "h6")
	return p
}()

func (a *Article) BeforeSave(_ *gorm.DB) error {
	a.Body = articleHTMLPolicy.Sanitize(a.Body)
	return nil
}

// Sanitize legacy rows on reads as well as new writes. No backfill is required
// before existing published articles can be served safely.
func (a *Article) AfterFind(_ *gorm.DB) error {
	a.Body = articleHTMLPolicy.Sanitize(a.Body)
	a.PublicAuthor = nil
	if a.Author != nil {
		a.PublicAuthor = &ArticleAuthor{ID: a.Author.ID, Name: a.Author.Name, FotoGuru: a.Author.FotoGuru}
	}
	return nil
}

// ArticleAuthor is the complete allowlist of author fields exposed by article
// APIs. The underlying User relationship is never serialized.
type ArticleAuthor struct {
	ID       uint    `gorm:"primaryKey" json:"id"`
	Name     string  `json:"name"`
	FotoGuru *string `json:"foto_guru,omitempty"`
}
