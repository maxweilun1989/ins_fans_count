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
	log.Printf("Ready to run: account %v", config.AccountCount)

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
		updateData(users, &appContext)
	}
}

func updateData(users []*instagram_fans.User, appContext *instagram_fans.AppContext) {
	if len(users) == 0 {
		return
	}

	if appContext.Config.AccountCount == 0 {
		return
	}

	var wg sync.WaitGroup
	var getAccountMutex sync.Mutex

	userChannel := make(chan *instagram_fans.User, len(users))
	for i := 0; i < len(users); i++ {
		userChannel <- users[i]
	}

	for i := 0; i < appContext.Config.AccountCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := UpdateUserInfo(userChannel, appContext, &getAccountMutex)
			if err != nil {
				log.Errorf("Update user info failed %v\n", err)
				return
			}
		}()
	}
	wg.Wait()
}

func UpdateUserInfo(userChannel <-chan *instagram_fans.User, appContext *instagram_fans.AppContext, mutex *sync.Mutex) error {

	var account *instagram_fans.Account
	var browser *playwright.Browser
	var page *playwright.Page
	for {
		_browser, err := instagram_fans.NewBrowser(appContext.Pw)
		if err != nil {
			log.Fatalf("Can not create browser, %v", err)
		}
		browser = _browser

		_page, err := instagram_fans.NewPage(browser)
		if err != nil {
			log.Fatalf("Can not create page, %v", err)
		}
		page = _page

		mutex.Lock()
		account = instagram_fans.FindAccount(appContext.AccountDb, appContext.Config.AccountTable, 1)
		mutex.Unlock()
		if account == nil {
			log.Errorf("No account avaliable")
			break
		}
		log.Printf("using account: %v", *account)
		if err := instagram_fans.LogInToInstagram(account, page, appContext.Config.DelayConfig.DelayAfterLogin); err != nil {
			log.Errorf("can not login to instagram, %v", err)
			instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, account, -1)
			(*page).Close()
			(*browser).Close()
			continue
		}
		break
	}
	defer (*browser).Close()
	defer (*page).Close()

	for user := range userChannel {
		user.FansCount = -2
		if appContext.Config.ParseFansCount {
			fansCount, err := instagram_fans.GetFansCount(page, user.Url)
			if err != nil {
				user.FansCount = fansCount
			}
		}
		if appContext.Config.ParseStoryLink {
			storyLink, err := instagram_fans.GetStoriesLink(page, user.Url, account)
			if err != nil {
				user.StoryLink = storyLink
			}
		}
		log.Printf("fans_count: %d, story_link: %s for %s", user.FansCount, user.StoryLink, user.Url)
		instagram_fans.UpdateSingleDataToDb(user, appContext)
		time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayForNext) * time.Millisecond)
	}
	return nil
}
