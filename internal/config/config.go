package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mrgolftech/Verdent/internal/account"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

type Runtime struct {
	Listen         string
	APIKey         string
	RequestTimeout time.Duration
	Protocol       protocol.Config
	Accounts       []account.Credential
}

type accountFile struct { Accounts []account.Credential `json:"accounts"` }

func Load() (Runtime,error) { return load(os.Getenv,os.ReadFile) }

func load(getenv func(string)string, readFile func(string)([]byte,error)) (Runtime,error) {
	r:=Runtime{
		Listen: strings.TrimSpace(getenv("VERDENT_LISTEN")),
		APIKey: getenv("VERDENT_API_KEY"),
		Protocol: protocol.Config{
			Endpoint:strings.TrimSpace(getenv("VERDENT_ENDPOINT")),
			CatalogEndpoint:strings.TrimSpace(getenv("VERDENT_CATALOG_ENDPOINT")),
			AppVersion:strings.TrimSpace(getenv("VERDENT_APP_VERSION")),
			BetaHeader:strings.TrimSpace(getenv("VERDENT_PROXY_BETA")),
			Sign:getenv("VERDENT_PROXY_SIGN"),
			UserAgent:strings.TrimSpace(getenv("VERDENT_USER_AGENT")),
			NativeAPI:true,
		},
	}
	if r.Listen=="" { r.Listen=":5084" }
	r.RequestTimeout=5*time.Minute
	if raw:=strings.TrimSpace(getenv("VERDENT_REQUEST_TIMEOUT"));raw!="" { d,err:=time.ParseDuration(raw);if err!=nil{return Runtime{},fmt.Errorf("VERDENT_REQUEST_TIMEOUT: %w",err)};if d<=0{return Runtime{},errors.New("VERDENT_REQUEST_TIMEOUT must be positive")};r.RequestTimeout=d }
	if r.Protocol.AppVersion=="" { return Runtime{},errors.New("VERDENT_APP_VERSION is required") }
	if r.Protocol.BetaHeader=="" { return Runtime{},errors.New("VERDENT_PROXY_BETA is required") }
	if r.Protocol.Sign=="" { return Runtime{},errors.New("VERDENT_PROXY_SIGN is required") }

	if path:=strings.TrimSpace(getenv("VERDENT_ACCOUNTS_FILE"));path!="" {
		data,err:=readFile(path);if err!=nil{return Runtime{},fmt.Errorf("read accounts file: %w",err)}
		var wrapped accountFile
		if err:=json.Unmarshal(data,&wrapped);err==nil && len(wrapped.Accounts)>0 { r.Accounts=wrapped.Accounts } else {
			var direct []account.Credential
			if err:=json.Unmarshal(data,&direct);err!=nil{return Runtime{},fmt.Errorf("decode accounts file: %w",err)}
			r.Accounts=direct
		}
	} else if token:=strings.TrimSpace(getenv("VERDENT_TOKEN"));token!="" {
		id:=strings.TrimSpace(getenv("VERDENT_ACCOUNT_ID"));if id==""{id="default"}
		r.Accounts=[]account.Credential{{
			ID:id,Label:strings.TrimSpace(getenv("VERDENT_ACCOUNT_LABEL")),Token:token,
			DeviceID:strings.TrimSpace(getenv("VERDENT_DEVICE_ID")),TeamID:strings.TrimSpace(getenv("VERDENT_TEAM_ID")),
			ProxyURL:strings.TrimSpace(getenv("VERDENT_PROXY")),
		}}
	}
	if len(r.Accounts)==0 { return Runtime{},errors.New("no Verdent accounts configured") }
	seen:=map[string]bool{}
	for i:=range r.Accounts {
		a:=&r.Accounts[i]
		a.ID=strings.TrimSpace(a.ID); a.Token=strings.TrimSpace(a.Token); a.DeviceID=strings.TrimSpace(a.DeviceID)
		if a.ID=="" { return Runtime{},fmt.Errorf("account %d: id is required",i) }
		if seen[a.ID] { return Runtime{},fmt.Errorf("duplicate account id %q",a.ID) };seen[a.ID]=true
		if a.Token=="" { return Runtime{},fmt.Errorf("account %q: token is required",a.ID) }
		if a.DeviceID=="" { return Runtime{},fmt.Errorf("account %q: device_id is required",a.ID) }
		if a.TeamID=="" { a.TeamID="0" }
	}
	return r,nil
}
