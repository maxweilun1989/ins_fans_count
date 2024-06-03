package main

import (
	"github.com/charmbracelet/log"
	"github.com/playwright-community/playwright-go"
	"instgram_fans/instagram_fans"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	log.Printf("start")

	config := instagram_fans.ParseConfig("config.json")
	if config == nil {
		log.Fatalf("Can not parse config!!!")
		return
	}

	db, err := instagram_fans.ConnectToDB(config.Dsn)
	if err != nil {
		log.Fatalf("failed to conenct to database %s, error: %v", config.Dsn, err)
	}
	defer instagram_fans.SafeCloseDB(db)

	accountDb, err := instagram_fans.ConnectToDB(config.AccountDSN)
	if err != nil {
		log.Fatalf("failed to conenct to database %s, error: %v", config.AccountDSN, err)
	}
	defer instagram_fans.SafeCloseDB(accountDb)

	accounts := instagram_fans.FindAccounts(accountDb, config.AccountTable, config.AccountCount)
	if len(accounts) == 0 {
		log.Fatalf("No account avaliable in database")
	}

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("Can not run playwright, %v", err)
	}
	defer func(pw *playwright.Playwright) {
		err := pw.Stop()
		if err != nil {
			log.Fatalf("Can not stop playwright, %v", err)
		}
	}(pw)

	appContext := instagram_fans.AppContext{Pw: pw, Db: db, AccountDb: accountDb, Config: config}
	log.Printf("Ready to run: account %v", accounts)

	low := 0
	for {
		users, err := instagram_fans.FindUserEmptyData(db, config.Table, config.Count, low)
		if err != nil {
			log.Fatalf("Can not find user empty data, %v", err)
		}
		if len(users) == 0 {
			log.Print("Done ALL! no data need to handle")
			break
		}

		begin := users[0].Id
		end := users[len(users)-1].Id
		log.Printf("has %d to handle, from %d to %d", len(users), begin, end)
		db.Table(config.Table).Where("id >= ? and id <= ?", begin, end).Updates(map[string]interface{}{"fans_count": -2})
		low = end
		updateData(users, &appContext, accounts)
	}
}

func updateData(users []*instagram_fans.User, appContext *instagram_fans.AppContext, accounts []*instagram_fans.Account) {
	if len(users) == 0 || len(accounts) == 0 {
		return
	}

	perSize := len(users) / len(accounts)
	var wg sync.WaitGroup
	for i, account := range accounts {
		begin := i * perSize
		end := (i + 1) * perSize
		if end > len(users) {
			end = len(users)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			err := UpdateUserInfo(users[begin:end], account, appContext)
			if err != nil {
				log.Errorf("Update user info failed %v\n", err)
				return
			}
		}()
	}
	wg.Wait()
}

func UpdateUserInfo(users []*instagram_fans.User,
	account *instagram_fans.Account,
	appContext *instagram_fans.AppContext) error {
	if len(users) == 0 {
		return nil
	}

	log.Printf("Start to update user info account(%v) id(%d~%d)", *account, users[0].Id, users[len(users)-1].Id)
	browser, err := instagram_fans.NewBrowser(appContext.Pw)
	if err != nil {
		log.Fatalf("Can not create browser, %v", err)
	}
	defer func(browser playwright.Browser) {
		err := browser.Close()
		if err != nil {

		}
	}(*browser)

	page, err := instagram_fans.NewPage(browser)
	if err != nil {
		log.Fatalf("Can not create page, %v", err)
	}
	defer func(page playwright.Page) {
		err := page.Close()
		if err != nil {

		}
	}(*page)

	if err := instagram_fans.LogInToInstagram(account, page); err != nil {
		log.Fatalf("can not login to instagram, %v", err)
	}

	time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayAfterLogin) * time.Millisecond)

	for _, user := range users {
		user.FansCount = -2
		if appContext.Config.ParseFansCount {
			user.FansCount = instagram_fans.GetFansCount(page, user.Url)
		}
		if appContext.Config.ParseStoryLink {
			user.StoryLink = instagram_fans.GetStoriesLink(page, user.Url, account)
		}
		log.Printf("fans_count: %d, story_link: %s for %s", user.FansCount, user.StoryLink, user.Url)
		instagram_fans.UpdateSingleDataToDb(user, appContext)
		time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayForNext) * time.Millisecond)
	}
	return nil
}
