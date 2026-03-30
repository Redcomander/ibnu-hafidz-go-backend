package models

import (
	"time"

	"gorm.io/gorm"
)

// ──────────────────────────────────────────────────────────────
// Album — container for gallery items
// ──────────────────────────────────────────────────────────────
type Album struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Title       string         `gorm:"type:varchar(255);not null" json:"title"`
	Slug        string         `gorm:"type:varchar(255);uniqueIndex" json:"slug"`
	Description string         `gorm:"type:text" json:"description"`
	CoverImage  *string        `gorm:"type:varchar(255)" json:"cover_image"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Galleries []Gallery `gorm:"foreignKey:AlbumID" json:"galleries,omitempty"`
}

func (Album) TableName() string { return "albums" }

// ──────────────────────────────────────────────────────────────
// Gallery — photo or video media item
// ──────────────────────────────────────────────────────────────
type Gallery struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	AlbumID     *uint          `gorm:"index" json:"album_id"`
	Title       string         `gorm:"type:varchar(255);not null" json:"title"`
	Type        string         `gorm:"type:enum('photo','video');not null" json:"type"`
	Path        string         `gorm:"type:varchar(500);not null" json:"path"`
	WebpPath    *string        `gorm:"type:varchar(500)" json:"webp_path"`
	Source      string         `gorm:"type:enum('upload','url','youtube','instagram','tiktok');default:'upload'" json:"source"`
	OriginalURL *string        `gorm:"column:original_url;type:varchar(500)" json:"original_url"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Album *Album `gorm:"foreignKey:AlbumID" json:"album,omitempty"`
}

func (Gallery) TableName() string { return "galleries" }

// ──────────────────────────────────────────────────────────────
// Category — article category
// ──────────────────────────────────────────────────────────────
type Category struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	Slug      string    `gorm:"type:varchar(255);uniqueIndex" json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships
	Articles []Article `gorm:"foreignKey:CategoryID" json:"articles,omitempty"`
}

func (Category) TableName() string { return "categories" }

// ──────────────────────────────────────────────────────────────
// Tag — article tag (many-to-many via article_tag pivot)
// ──────────────────────────────────────────────────────────────
type Tag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	Slug      string    `gorm:"type:varchar(255);uniqueIndex" json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships
	Articles []Article `gorm:"many2many:article_tag;" json:"articles,omitempty"`
}

func (Tag) TableName() string { return "tags" }

// ──────────────────────────────────────────────────────────────
// Article — blog post / berita
// ──────────────────────────────────────────────────────────────
type Article struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Title       string     `gorm:"type:varchar(255);not null" json:"title"`
	Slug        string     `gorm:"type:varchar(255);uniqueIndex;not null" json:"slug"`
	Body        string     `gorm:"type:longtext" json:"body"`
	Excerpt     *string    `gorm:"type:text" json:"excerpt"`
	Thumbnail   *string    `gorm:"type:varchar(255)" json:"thumbnail"`
	CategoryID  *uint      `gorm:"index" json:"category_id"`
	AuthorID    uint       `gorm:"column:author_id;not null" json:"author_id"`
	Status      string     `gorm:"type:enum('draft','published');default:'draft'" json:"status"`
	PublishedAt *time.Time `json:"published_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	// Relationships
	Category *Category     `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	Author   *User         `gorm:"foreignKey:AuthorID" json:"author,omitempty"`
	Tags     []Tag         `gorm:"many2many:article_tag;" json:"tags,omitempty"`
	Views    []ArticleView `gorm:"foreignKey:ArticleID" json:"-"`
	Comments []Comment     `gorm:"foreignKey:ArticleID" json:"comments,omitempty"`
}

func (Article) TableName() string { return "articles" }

// ──────────────────────────────────────────────────────────────
// ArticleView — tracks page views per article
// ──────────────────────────────────────────────────────────────
type ArticleView struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ArticleID uint      `gorm:"not null;index" json:"article_id"`
	UserID    *uint     `json:"user_id"`
	IPAddress string    `gorm:"column:ip_address;type:varchar(45)" json:"ip_address"`
	UserAgent string    `gorm:"column:user_agent;type:text" json:"user_agent"`
	Referrer  *string   `gorm:"type:varchar(500)" json:"referrer"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships
	Article *Article `gorm:"foreignKey:ArticleID" json:"article,omitempty"`
}

func (ArticleView) TableName() string { return "article_views" }

// ──────────────────────────────────────────────────────────────
// Comment — article comment with nested replies
// ──────────────────────────────────────────────────────────────
type Comment struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ArticleID      uint      `gorm:"not null;index" json:"article_id"`
	ParentID       *uint     `gorm:"index" json:"parent_id"`
	Name           string    `gorm:"type:varchar(255);not null" json:"name"`
	Email          string    `gorm:"type:varchar(255)" json:"email"`
	Body           string    `gorm:"type:text;not null" json:"body"`
	LikesCount     int       `gorm:"default:0" json:"likes_count"`
	ProfilePicture *string   `gorm:"type:varchar(500)" json:"profile_picture"`
	GoogleID       *string   `gorm:"type:varchar(255)" json:"google_id"`
	IsApproved     bool      `gorm:"default:false" json:"is_approved"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Relationships
	Article *Article  `gorm:"foreignKey:ArticleID" json:"article,omitempty"`
	Parent  *Comment  `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Replies []Comment `gorm:"foreignKey:ParentID" json:"replies,omitempty"`
}

func (Comment) TableName() string { return "comments" }
