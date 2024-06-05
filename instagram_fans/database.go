package instagram_fans

import (
	"bufio"
	"github.com/charmbracelet/log"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
)

type Account struct {
	Username    string `gorm:"column:user"`
	Password    string `gorm:"column:psw"`
	Status      int    `gorm:"column:status"`
	MachineCode string `gorm:"column:Machine_code"`
}

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

func UsableAccountCount(db *gorm.DB, table string) int {
	var count int64
	result := db.Table(table).Where("status = 0").Count(&count)
	if result.Error != nil {
		log.Errorf("Can not get account count, %v", result.Error)
		return 0
	}
	return int(count)
}

func FindAccount(db *gorm.DB, table string, machineCode string) *Account {
	var accounts []Account
	result := db.Table(table).Where("status = 0").Order("id ASC").Find(&accounts)
	if result.Error != nil {
		log.Errorf("Can not find account, %v", result.Error)
		return nil
	}
	if len(accounts) == 0 {
		log.Errorf("No account found")
		return nil
	}

	var account *Account
	for _, cur := range accounts {
		if cur.MachineCode == machineCode {
			account = &cur
			break
		}
	}
	if account == nil {
		account = &accounts[0]
	}
	return account
}

func MarkAccountStatus(db *gorm.DB, table string, account *Account, status int, machineCode string) {
	log.Infof("MarkAccountStatus %s to %d", account.Username, status)
	result := db.Table(table).Where("user = ?", account.Username).Updates(map[string]interface{}{"status": status, "Machine_code": machineCode})
	if result.Error != nil {
		log.Errorf("Can not update account status, %v", result.Error)
	}
}

func SetAccountMachineCode(db *gorm.DB, table string, account *Account, machineCode string) {
	result := db.Table(table).Where("user = ?", account.Username).Updates(map[string]interface{}{"Machine_code": machineCode})
	if result.Error != nil {
		log.Errorf("Can not update account status, %v", result.Error)
	}
}

func FindUserEmptyData(db *gorm.DB, table string, limit int, low int) ([]*User, error) {

	var users []*User
	db.Table(table).Where("fans_count = -1").Where("id > ?", low).Order("id ASC").Limit(limit).Find(&users)
	return users, nil
}

func MarkUserStatusIsWorking(users []*User, db *gorm.DB, table string) {
	begin := users[0].Id
	end := users[len(users)-1].Id
	log.Infof("has %d to handle, from %d to %d", len(users), begin, end)
	db.Table(table).
		Where("id >= ? and id <= ?", begin, end).
		Where("fans_count = -1").
		Updates(map[string]interface{}{"fans_count": -2})
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
