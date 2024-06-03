package main

import (
	"github.com/charmbracelet/log"
	"github.com/pkg/errors"
	"github.com/playwright-community/playwright-go"
	"instgram_fans/instagram_fans"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type PageContext struct {
	Browser *playwright.Browser
	Page    *playwright.Page
	Account *instagram_fans.Account
}

func (p *PageContext) Close() {
	if p.Page != nil {
		(*p.Page).Close()
	}
	if p.Browser != nil {
		(*p.Browser).Close()
	}
}

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

	userChannel := make(chan *instagram_fans.User, len(users)+appContext.Config.AccountCount)
	for i := 0; i < len(users); i++ {
		userChannel <- users[i]
	}
	for i := 0; i < appContext.Config.AccountCount; i++ {
		userChannel <- nil
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

	pageContext, err := getLoginPage(appContext, mutex)
	if err != nil {
		return err
	}
	defer pageContext.Close()

	for user := range userChannel {
		if user == nil {
			instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, 0)
			log.Info("Receive nil!!")
			break
		}
		user.FansCount = -2
		var fetchErr error

		for {
			if appContext.Config.ParseFansCount {
				fansCount, err := instagram_fans.GetFansCount(pageContext.Page, user.Url)
				if err == nil {
					user.FansCount = fansCount
				}
				fetchErr = err
			}
			if appContext.Config.ParseStoryLink {
				storyLink, err := instagram_fans.GetStoriesLink(pageContext.Page, user.Url, pageContext.Account)
				if err == nil {
					user.StoryLink = storyLink
				}
				fetchErr = err
			}

			if errors.Is(fetchErr, instagram_fans.ErrUserInvalid) {
				instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, -1)
				pageContext.Close()
				log.Errorf("enconter err use invalid: %v, login again", fetchErr)
				time.Sleep(time.Duration(5) * time.Second)
				_pageContext, err := getLoginPage(appContext, mutex)
				if err != nil {
					return err
				}
				pageContext = _pageContext
				continue
			}

			break
		}

		log.Printf("fans_count: %d, story_link: %s for %s", user.FansCount, user.StoryLink, user.Url)
		instagram_fans.UpdateSingleDataToDb(user, appContext)
		time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayForNext) * time.Millisecond)
	}
	return nil
}

func getLoginPage(appContext *instagram_fans.AppContext, mutex *sync.Mutex) (*PageContext, error) {
	var pageContext PageContext

	for {
		browser, err := instagram_fans.NewBrowser(appContext.Pw)
		if err != nil {
			time.Sleep(time.Duration(2) * time.Second)
			log.Errorf("Can not create browser, %v", err)
			return nil, errors.Wrap(err, "Can not create browser")
		}
		pageContext.Browser = browser

		page, err := instagram_fans.NewPage(browser)
		if err != nil {
			time.Sleep(time.Duration(2) * time.Second)
			(*browser).Close()
			return nil, errors.Wrap(err, "Can not create page")
		}
		pageContext.Page = page

		mutex.Lock()
		account := instagram_fans.FindAccount(appContext.AccountDb, appContext.Config.AccountTable, 1)
		mutex.Unlock()
		if account == nil {
			log.Errorf("No account avaliable")
			pageContext.Close()
			return nil, errors.New("No account")
		}
		log.Printf("using account: %v", *account)
		if err := instagram_fans.LogInToInstagram(account, page, appContext.Config.DelayConfig.DelayAfterLogin); err != nil {
			log.Errorf("can not login to instagram, %v", err)
			instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, account, -1)
			(*page).Close()
			(*browser).Close()
			continue
		}
		pageContext.Account = account
		break
	}

	return &pageContext, nil
}
