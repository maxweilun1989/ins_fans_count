package main

import (
	"github.com/charmbracelet/log"
	"github.com/emirpasic/gods/sets/treeset"
	"github.com/petermattis/goid"
	"github.com/pkg/errors"
	"github.com/playwright-community/playwright-go"
	"gorm.io/gorm"
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
	goId    int64
}

func (p *PageContext) Close() {
	if p.Page != nil {
		err := (*p.Page).Close()
		if err != nil {
			return
		}
	}
	if p.Browser != nil {
		err := (*p.Browser).Close()
		if err != nil {
			return
		}
	}
	log.Infof("[%d] Context is closed for account[%s]", p.goId, p.Account.Username)
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

	instagram_fans.MakAccountUsable(appContext.AccountDb, appContext.Config.AccountTable, appContext.MachineCode)
	// 计算可以使用的账号
	finalAccountCount := computeAccountCount(appContext)
	if finalAccountCount == 0 {
		log.Errorf("No account available, exit !!!!")
		return
	}

	if err = updateData(appContext, finalAccountCount); err != nil {
		log.Errorf("Update data failed %v", err)
		return
	}
}

func updateData(appContext *instagram_fans.AppContext, count int) error {
	var wg sync.WaitGroup
	var markAccountMutex sync.Mutex

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := UpdateUserInfo(appContext, &markAccountMutex)
			if err != nil {
				log.Errorf("Update user info failed %v\n", err)
				return
			}
		}()
	}
	wg.Wait()
	return nil
}

func UpdateUserInfo(appContext *instagram_fans.AppContext, mutex *sync.Mutex) error {

	low := 0
	db := appContext.Db
	config := appContext.Config

	var pageContext *PageContext
	set := treeset.NewWithIntComparator()
	count := 0

	for {
		mutex.Lock()
		users, err := fetchBloggerToHandle(db, config, low)
		if err != nil {
			log.Errorf("Can not find user empty data, %v", err)
			mutex.Unlock()
			return errors.Wrap(err, "Can not find user empty data")
		}
		mutex.Unlock()
		if len(users) == 0 {
			log.Infof("Done for this browser, no data to handle")
			break
		}

		log.Infof("find %d users for (%d ~ %d)!!!", len(users), users[0].Id, users[len(users)-1].Id)
		low = users[len(users)-1].Id

		set.Clear()
		for _, user := range users {
			set.Add(user.Id)
		}

		for _, user := range users {
		ChooseAccountAndLogin:
			// 创建pageContext，找到一个可用的账号并登录成功
			log.Infof("ChooseAccountAndLogin, context(%v)", pageContext)
			if pageContext == nil {
				pageContext, err = initPageContext(appContext, mutex)
				if err != nil {
					log.Errorf("Can not init page context, %v", err)
					if !set.Empty() {
						values := set.Values()
						begin := values[0].(int)
						end := values[len(values)-1].(int)
						log.Errorf("Some users are not handled(%d ~ %d)", begin, end)
						instagram_fans.MarkUserStatusIdle(begin, end, db, config.Table)
					}
					pageContext = nil
					return err
				}
				log.Infof("start to fetch data using account %s (%d - %d)", pageContext.Account.Username, users[0].Id, users[len(users)-1].Id)
			}

		FetchData:
			fetchErr := fetchBloggerData(appContext, pageContext, user)

			if fetchErr != nil {
				status := handleFetchErr(fetchErr, appContext, pageContext)
				if status == StatusNeedAnotherAccount {
					pageContext = nil
					goto ChooseAccountAndLogin
				} else if status == StatusCanRefetch {
					time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayAfterLogin) * time.Second)
					goto FetchData
				} else if status == StatusNext {
					time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayForNext) * time.Second)
					continue
				}
			}

			set.Remove(user.Id)
			log.Infof("[%d] fans_count: %d, story_link: %s for %s", pageContext.goId, user.FansCount, user.StoryLink, user.Url)
			instagram_fans.UpdateSingleDataToDb(user, appContext)

			time.Sleep(time.Duration(appContext.Config.DelayConfig.DelayForNext) * time.Millisecond)

			count++
			log.Infof("count[%s] handle %d blogger", pageContext.Account.Username, count)

			if count >= config.MaxCount {
				pageContext.Close()
				pageContext = nil
				count = 0
				goto ChooseAccountAndLogin
			}
		}
	}

	if pageContext != nil {
		if pageContext.Account != nil {
			instagram_fans.MarkAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, 0, appContext.MachineCode)
		}
		pageContext.Close()
	}

	return nil
}

func fetchBloggerToHandle(db *gorm.DB, config *instagram_fans.Config, low int) ([]*instagram_fans.User, error) {
	users, err := instagram_fans.FindBloger(db, config.Table, config.Count, low)
	if err != nil {
		log.Errorf("Can not find user empty data, %v", err)
		return nil, errors.Wrap(err, "Can not find user empty data")
	}

	if len(users) == 0 {
		log.Infof("Done ALL! no data need to handle")
		return users, nil
	}
	instagram_fans.MarkUserStatusIsWorking(users, db, config.Table)
	return users, nil
}

func fetchBloggerData(appContext *instagram_fans.AppContext, pageContext *PageContext, user *instagram_fans.User) error {
	user.FansCount = -2

	if appContext.Config.ParseFansCount {
		fansCount, err := instagram_fans.GetFansCount(pageContext.Page, user.Url)
		if err != nil {
			return err
		}

		user.FansCount = fansCount
	}
	if appContext.Config.ParseStoryLink {
		storyLink, err := instagram_fans.GetStoriesLink(pageContext.Page, user.Url)
		if err != nil {
			return err
		}
		user.StoryLink = storyLink
	}
	return nil
}

func initPageContext(appContext *instagram_fans.AppContext, mutex *sync.Mutex) (*PageContext, error) {
	for {
		mutex.Lock()
		account := instagram_fans.FindAccount(appContext.AccountDb, appContext.Config.AccountTable, appContext.MachineCode)
		if account != nil {
			instagram_fans.MarkAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, account, 1, appContext.MachineCode)
		}
		mutex.Unlock()

		if account == nil {
			instagram_fans.MakAccountUsable(appContext.AccountDb, appContext.Config.AccountTable, appContext.MachineCode)
			return nil, errors.New("No account available!!")
		}

		pageContext, err := getLoginPageContext(appContext, account)
		if err != nil {
			log.Errorf("Can not get login in mark user(%s) to -1/-2 ", account.Username)
			if errors.Is(err, instagram_fans.ErrUserUnusable) {
				instagram_fans.MarkAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, account, -2, appContext.MachineCode)
			} else {
				instagram_fans.MarkAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, account, -1, appContext.MachineCode)
			}

			if pageContext != nil {
				pageContext.Close()
			}
			pageContext = nil
			continue
		}
		pageContext.goId = goid.Get()
		return pageContext, nil
	}
}

func getLoginPageContext(appContext *instagram_fans.AppContext, account *instagram_fans.Account) (*PageContext, error) {
	var pageContext PageContext
	pageContext.goId = goid.Get()
	pageContext.Account = account

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
		log.Errorf("[%d] getLoginPage Can not login to instagram!!! %v", pageContext.goId, err)
		return &pageContext, err
	}

	return &pageContext, nil
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
		log.Errorf("[handleFetchErr] [%d] enconter err need ChooseAccountAndLogin: %v, ChooseAccountAndLogin again, account [%v]", pageContext.goId, fetchErr, pageContext.Account)
		if errors.Is(fetchErr, instagram_fans.ErrUserUnusable) {
			instagram_fans.MarkAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, -2, appContext.MachineCode)
		} else {
			instagram_fans.MarkAccountStatus(appContext.AccountDb, appContext.Config.AccountTable, pageContext.Account, -1, appContext.MachineCode)
		}
		pageContext.Close()
		pageContext = nil
		return StatusNeedAnotherAccount
	}

	if errors.Is(fetchErr, instagram_fans.ErrNeedLogin) {
		log.Errorf("[handleFetchErr] %s enconter err need relogin: %v, relogin again", pageContext.Account.Username, fetchErr)
		if err := instagram_fans.LogInToInstagram(pageContext.Account, pageContext.Page); err != nil {
			pageContext.Close()
			pageContext = nil
			return StatusNeedAnotherAccount
		}
		log.Infof("[handleFetchErr] relogin success, continue fetch data %v", *pageContext.Account)
		return StatusCanRefetch
	}

	if errors.Is(fetchErr, instagram_fans.ErrPageUnavailable) {
		log.Errorf("[handleFetchErr] page unavailable %v", fetchErr)
		return StatusNext
	}
	return StatusNext
}
