package instagram_fans

type User struct {
	Id        int    `gorm:"primaryKey"`
	Url       string `gorm:"unique"`
	StoryLink string `gorm:"default:null"`
	FansCount int    `gorm:"default:-1"`
}

var similarUserTableName string

type UserSimilarFriends struct {
	Id             int    `gorm:"primaryKey"`
	OwnerUrl       string `gorm:"unique"`
	SimilarFriends string `gorm:"default:null;size:1024"`
	Status         int    `gorm:"default:0"`
}

func (UserSimilarFriends) TableName() string {
	return similarUserTableName
}
