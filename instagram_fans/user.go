package instagram_fans

type User struct {
	Id        int    `gorm:"primaryKey"`
	Url       string `gorm:"unique"`
	StoryLink string `gorm:"default:null"`
	FansCount int    `gorm:"default:-1"`
}
