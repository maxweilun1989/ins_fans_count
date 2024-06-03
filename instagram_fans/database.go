package instagram_fans

import (
	"bufio"
	"github.com/charmbracelet/log"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
)

func ConnectToDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Can not open db(%s), %v", dsn, err)
	}
	return db, err
}

func SafeCloseDB(db *gorm.DB) {
	sqlDb, err := db.DB()
	if err != nil {
		log.Fatalf("Can not get db, %v", err)
	}
	if err := sqlDb.Close(); err != nil {
		log.Fatalf("Can not close db, %v", err)
	}
}
func FindUserEmptyData(db *gorm.DB, table string, limit int, low int) ([]*User, error) {

	var users []*User
	db.Table(table).Where("fans_count = -1").Where("id > ?", low).Order("id ASC").Limit(limit).Find(&users)
	// queryStr := fmt.Sprintf("SELECT id, url FROM %s WHERE fans_count = -1 and id > %d order by id ASC limit %d", table, low, limit)
	return users, nil
}

func UpdateSingleDataToDb(user *User, appContext *AppContext) {
	if !appContext.Config.ParseFansCount && !appContext.Config.ParseStoryLink {
		log.Errorf("No parseFansCount and parseStoryLink found in config")
		return
	}
	db := appContext.Db
	table := appContext.Config.Table

	if appContext.Config.ParseFansCount && appContext.Config.ParseStoryLink {
		if user.FansCount == -2 && user.StoryLink == "" {
			log.Errorf("No fans count and story link found in user(%s)", user.Url)
			return
		}
		db.Table(table).Where("url = ?", user.Url).Updates(map[string]interface{}{"story_link": user.StoryLink, "fans_count": user.FansCount})
		// execStr := fmt.Sprintf("UPDATE %s SET story_link = ?, fans_count = ? WHERE url = ?", table)
		// _, err = db.Exec(execStr, user.StoryLink, user.FansCount, user.Url)
	} else if appContext.Config.ParseFansCount && !appContext.Config.ParseStoryLink {
		if user.FansCount == -2 {
			log.Errorf("No fans count found in user(%s)", user.Url)
			return
		}
		db.Table(table).Where("url = ?", user.Url).Updates(map[string]interface{}{"fans_count": user.FansCount})
		// execStr := fmt.Sprintf("UPDATE %s SET fans_count = ? WHERE url = ?", table)
		// _, err = db.Exec(execStr, user.FansCount, user.Url)
	} else if !appContext.Config.ParseFansCount && appContext.Config.ParseStoryLink {
		if user.StoryLink == "" {
			log.Errorf("No story link found in user(%s)", user.Url)
			return
		}
		db.Table(table).Where("url = ?", user.Url).Updates(map[string]interface{}{"story_link": user.StoryLink})
		// execStr := fmt.Sprintf("UPDATE %s SET story_link = ? WHERE url = ?", table)
		// _, err = db.Exec(execStr, user.StoryLink, user.Url)
	}

	log.Printf("update user(%s) count %d, link: %s success", user.Url, user.FansCount, user.StoryLink)
}

func InsertFilesToDb(path string, dsn string) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Can not open file(%s), %v", path, err)
	}
	defer file.Close()

	db, err := ConnectToDB(dsn)
	if err != nil {
		log.Fatalf("Can not connect to db, %v", err)
	}
	defer SafeCloseDB(db)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("Read line: %s", line)

		db.Table("user").Save(&User{Url: line})
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Can not read file(%s), %v", path, err)
	}
}
