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

	appContext, err := instagram_fans.InitContext()
	if err != nil {
		log.Fatalf("Can not init context, %v", err)
		return
	}
	defer appContext.DestroyContext()

	db := appContext.Db
	config := appContext.Config

	low := 0
	for {
		users, err := instagram_fans.FindUserEmptyData(db, config.Table, config.Count, low)
		if err != nil {
			log.Fatalf("Can not find user empty data, %v", err)
			break
		}
		if len(users) == 0 {
			log.Infof("Done ALL! no data need to handle")
			break
		}
		instagram_fans.MarkUserStatusIsWorking(users, db, config.Table)
		if err := updateData(users, appContext); err != nil {
			log.Errorf("Update data failed %v", err)
			break
		}
	}
}

func updateData(users []*instagram_fans.User, appContext *instagram_fans.AppContext) error {

	// 计算可以使用的账号
	finalAccountCount := computeAccountCount(appContext)
	if finalAccountCount == 0 {
		return errors.New("No account available!!")
	}

	var wg sync.WaitGroup
	var getAccountMutex sync.Mutex

	// 准备好数据
	userChannel := prepareUserChannel(users, finalAccountCount)

	for i := 0; i < finalAccountCount; i++ {
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
			instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, 0, appContext.MachineCode)
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

			if errors.Is(fetchErr, instagram_fans.ErrUserInvalid) || errors.Is(fetchErr, instagram_fans.ErrUserUnusable) {
				if errors.Is(fetchErr, instagram_fans.ErrUserUnusable) {
					instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, -2, appContext.MachineCode)
				} else {
					instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, -1, appContext.MachineCode)
				}
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

			if errors.Is(fetchErr, instagram_fans.ErrNeedLogin) {
				log.Errorf("enconter err need login: %v, login again", fetchErr)
				time.Sleep(time.Duration(5) * time.Second)
				if err := instagram_fans.LogInToInstagram(pageContext.Account, pageContext.Page, appContext.Config.DelayConfig.DelayAfterLogin); err != nil {
					pageContext.Close()

					time.Sleep(time.Duration(5) * time.Second)

					_pageContext, err := getLoginPage(appContext, mutex)
					if err != nil {
						return err
					}
					pageContext = _pageContext
				}
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
		account := instagram_fans.FindAccount(appContext.AccountDb, appContext.Config.AccountTable, appContext.MachineCode)
		mutex.Unlock()

		if account == nil {
			log.Errorf("No account avaliable")
			pageContext.Close()
			return nil, errors.New("No account")
		}
		log.Printf("using account: %v", *account)
		if err := instagram_fans.LogInToInstagram(account, page, appContext.Config.DelayConfig.DelayAfterLogin); err != nil {
			log.Errorf("can not login to instagram, %v", err)
			instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, account, -1, appContext.MachineCode)
			(*page).Close()
			(*browser).Close()
			continue
		}
		pageContext.Account = account
		break
	}

	return &pageContext, nil
}

func prepareUserChannel(users []*instagram_fans.User, finalAccountCount int) chan *instagram_fans.User {
	userChannel := make(chan *instagram_fans.User, len(users)+finalAccountCount)
	for i := 0; i < len(users); i++ {
		userChannel <- users[i]
	}
	for i := 0; i < finalAccountCount; i++ {
		userChannel <- nil
	}
	return userChannel
}

func computeAccountCount(appContext *instagram_fans.AppContext) int {
	usableAccountCount := instagram_fans.UsableAccountCount(appContext.AccountDb, appContext.Config.AccountTable)
	if usableAccountCount == 0 {
		log.Errorf("No Count Avaliable found in account table")
		return 0
	}

	finalAccountCount := min(usableAccountCount, appContext.Config.AccountCount)
	return finalAccountCount
}
