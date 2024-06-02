package main

import (
	"database/sql"
	"fmt"
	"github.com/charmbracelet/log"
	"github.com/playwright-community/playwright-go"
	"instgram_fans/instagram_fans"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// update user set fans_count = -1 where story_link = "" or story_link is null;
// view-source:https://www.instagram.com/stories/cl3milson/
//

func main() {
	log.Printf("start")

	config, err := instagram_fans.ParseConfig("config.json")
	if err != nil {
		log.Fatalf("Can not parse config, %v", err)
	}
	if len(config.Accounts) == 0 {
		log.Fatalf("No account found")

	}

	db, err := instagram_fans.ConnectToDB(config.Dsn)
	if err != nil {
		log.Fatalf("failed to conenct to database %s, error: %v", config.Dsn, err)
	}
	defer db.Close()

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("Can not run playwright, %v", err)
	}
	defer pw.Stop()

	low := 0
	limit := 500

	for {
		users, err := instagram_fans.FindUserEmptyData(db, config.Table, limit, low)
		if err != nil {
			log.Fatalf("Can not find user empty data, %v", err)
		}
		if len(users) == 0 {
			break
		}

		begin := users[0].Id
		end := users[len(users)-1].Id
		log.Printf("has %d to handle, from %d to %d", len(users), begin, end)
		updateStr := fmt.Sprintf("UPDATE %s SET fans_count = -2 WHERE id >=  ? and id <= ? ", config.Table)
		_, updateErr := db.Exec(updateStr, begin, end)
		if updateErr != nil {
			log.Printf("Can not update db for %s ", updateErr)
		}
		low = end
		updateData(users, config, pw, db)
	}
}

func updateData(users []*instagram_fans.User, config *instagram_fans.Config, pw *playwright.Playwright, db *sql.DB) {
	if len(users) == 0 {
		return
	}

	perSize := len(users) / len(config.Accounts)
	var wg sync.WaitGroup
	for i, account := range config.Accounts {
		begin := i * perSize
		end := (i + 1) * perSize
		if end > len(users) {
			end = len(users)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			UpdateUserInfo(users[begin:end], account, config, pw, db)
		}()
	}
	wg.Wait()
}

func UpdateUserInfo(users []*instagram_fans.User, account instagram_fans.Account, config *instagram_fans.Config, pw *playwright.Playwright, db *sql.DB) {
	context, err := instagram_fans.LogInToInstagram(account.Username, account.Password, pw, config)
	if err != nil {
		log.Fatalf("Can not Login to instagram, %v", err)
	}
	defer (*context.Browser).Close()

	for _, user := range users {
		user.FansCount = instagram_fans.GetFansCount(context.Page, user.Url)
		user.StoryLink = instagram_fans.GetStoriesLink(context.Page, user.Url, &account, config)
		log.Printf("fans_count: %d, story_link: %s for %s", user.FansCount, user.StoryLink, user.Url)
		instagram_fans.UpdateSingleDataToDb(user, db, config.Table)
		time.Sleep(time.Duration(config.DelayConfig.DelayForNext) * time.Millisecond)
	}
}
