package instagram_fans

import (
	"github.com/charmbracelet/log"
	"github.com/pkg/errors"
	"github.com/playwright-community/playwright-go"
	"gorm.io/gorm"
)

type AppContext struct {
	Pw          *playwright.Playwright
	Db          *gorm.DB
	AccountDb   *gorm.DB
	Config      *Config
	MachineCode string
}

var (
	ErrorParseConfig      = errors.New("Can not parse config!!!")
	ErrorGenerateUUID     = errors.New("Can not get or generate uuid!!!")
	ErrorConnectDB        = errors.New("Can not connect to database!!!")
	ErrorConnectAccountDB = errors.New("Can not connect to account database!!!")
	ErrorPlayWrightStart  = errors.New("Can not start playwright!!!")
)

func InitContext() (*AppContext, error) {

	config := ParseConfig("config.json")
	if config == nil {
		return nil, ErrorParseConfig
	}

	machineCode, err := GetOrGenerateUUID("uuid.txt")
	if err != nil {
		return nil, ErrorGenerateUUID
	}
	log.Infof("Machine code: %s", machineCode)

	db, err := ConnectToDB(config.Dsn)
	if err != nil {
		return nil, ErrorConnectDB
	}
	log.Infof("Connect to db(%s) success", config.Dsn)

	accountDb, err := ConnectToDB(config.AccountDSN)
	if err != nil {
		return nil, ErrorConnectAccountDB
	}
	log.Infof("Connect to account db(%s) success", config.AccountDSN)

	pw, err := playwright.Run()
	if err != nil {
		return nil, ErrorPlayWrightStart
	}

	appContext := AppContext{Pw: pw, Db: db, AccountDb: accountDb, Config: config, MachineCode: machineCode}
	return &appContext, nil
}

func (appContext *AppContext) DestroyContext() {
	if appContext.Db != nil {
		SafeCloseDB(appContext.Db)
		appContext.Db = nil
	}
	if appContext.AccountDb != nil {
		SafeCloseDB(appContext.AccountDb)
	}

	if appContext.Pw != nil {
		err := appContext.Pw.Stop()
		if err != nil {
			log.Fatalf("Can not stop playwright, %v", err)
		}
		appContext.Pw = nil
	}
}
