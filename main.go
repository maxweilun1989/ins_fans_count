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

var (
	StatusNeedAnotherAccount = 1 // StatusNeedAnotherAccount 需要重新选择另外一个账号登录
	StatusCanRefetch         = 2 // StatusCanRefetch 重新登录成功，可以重新获取数据
	StatusNext               = 3
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
	p.Account = nil
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

		// 计算可以使用的账号
		finalAccountCount := computeAccountCount(appContext)
		if finalAccountCount == 0 {
			break
		}

		instagram_fans.MarkUserStatusIsWorking(users, db, config.Table)
		if err := updateData(users, appContext, finalAccountCount); err != nil {
			log.Errorf("Update data failed %v", err)
			break
		}
	}
}

func updateData(users []*instagram_fans.User, appContext *instagram_fans.AppContext, count int) error {

	var wg sync.WaitGroup
	var getAccountMutex sync.Mutex

	// 准备好数据
	userChannel := prepareUserChannel(users, count)

	for i := 0; i < count; i++ {
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
	return nil
}

func UpdateUserInfo(userChannel <-chan *instagram_fans.User, appContext *instagram_fans.AppContext, mutex *sync.Mutex) error {
	var pageContext *PageContext
	var err error

	for user := range userChannel {
		if user == nil {
			break
		}

	ChooseAccountAndLogin:
		// 创建pageContext，找到一个可用的账号并登录成功
		if pageContext == nil {
			mutex.Lock()
			account := instagram_fans.FindAccount(appContext.AccountDb, appContext.Config.AccountTable, appContext.MachineCode)
			mutex.Unlock()

			if account == nil {
				err = errors.New("No account available!!")
				break
			}

			pageContext, err = getLoginPageContext(appContext, account)
			if err != nil {
				if pageContext != nil {
					pageContext.Close()
				}
				pageContext = nil
				continue
			}
		}

		instagram_fans.SetAccountMachineCode(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, appContext.MachineCode)

	FetchData:
		user.FansCount = -2
		var fetchErr error

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

		if fetchErr != nil {
			status := handleFetchErr(fetchErr, appContext, pageContext)
			if status == StatusNeedAnotherAccount {
				goto ChooseAccountAndLogin
			} else if status == StatusCanRefetch {
				goto FetchData
			} else if status == StatusNext {
				continue
			}
		}

		log.Printf("fans_count: %d, story_link: %s for %s", user.FansCount, user.StoryLink, user.Url)
		instagram_fans.UpdateSingleDataToDb(user, appContext)
		time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayForNext) * time.Millisecond)
	}

	if pageContext != nil {
		if pageContext.Account != nil {
			instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, 0, appContext.MachineCode)
		}
		pageContext.Close()
	}

	return nil
}

func getLoginPageContext(appContext *instagram_fans.AppContext, account *instagram_fans.Account) (*PageContext, error) {
	var pageContext PageContext

	browser, err := instagram_fans.NewBrowser(appContext.Pw)
	if err != nil {
		return &pageContext, errors.Wrap(err, "Can not create browser!!!")
	}
	pageContext.Browser = browser

	page, err := instagram_fans.NewPage(browser)
	if err != nil {
		return &pageContext, errors.Wrap(err, "Can not create page!!!")
	}
	pageContext.Page = page

	log.Printf("using account: %v", *account)

	if err := instagram_fans.LogInToInstagram(account, page); err != nil {
		instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, account, -1, appContext.MachineCode)
		return &pageContext, errors.New("Can not login to instagram!!!")
	}
	pageContext.Account = account

	time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayAfterLogin) * time.Second)
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

func handleFetchErr(fetchErr error, appContext *instagram_fans.AppContext, pageContext *PageContext) int {
	if errors.Is(fetchErr, instagram_fans.ErrUserInvalid) || errors.Is(fetchErr, instagram_fans.ErrUserUnusable) {
		log.Errorf("enconter err need ChooseAccountAndLogin: %v, ChooseAccountAndLogin again", fetchErr)
		if errors.Is(fetchErr, instagram_fans.ErrUserUnusable) {
			instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, -2, appContext.MachineCode)
		} else {
			instagram_fans.MakeAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, -1, appContext.MachineCode)
		}
		pageContext.Close()
		pageContext = nil
		return StatusNeedAnotherAccount
	}

	if errors.Is(fetchErr, instagram_fans.ErrNeedLogin) {
		log.Errorf("enconter err need relogin: %v, relogin again", fetchErr)
		time.Sleep(time.Duration(5) * time.Second)
		if err := instagram_fans.LogInToInstagram(pageContext.Account, pageContext.Page); err != nil {
			pageContext.Close()
			pageContext = nil
			return StatusNeedAnotherAccount
		}
		log.Infof("relogin success, continue fetch data %v", *pageContext.Account)
		time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayAfterLogin) * time.Second)
	}

	if errors.Is(fetchErr, instagram_fans.ErrPageUnavailable) {
		log.Errorf("page unavailable %v", fetchErr)
		return StatusNext
	}
	return StatusNext
}
