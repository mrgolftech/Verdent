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
	AdminUser      string
	AdminPassword  string
	AccountsFile   string
	PublicBaseURL  string
	AuthBaseURL    string
	LoginBaseURL   string
	RequestTimeout time.Duration
	Protocol       protocol.Config
	Accounts       []account.Credential
}

type storedCredential struct {
	ID             string `json:"id"`
	Label          string `json:"label,omitempty"`
	Token          string `json:"token"`
	RefreshToken   string `json:"refresh_token,omitempty"`
	TokenExpiresAt int64  `json:"token_expires_at,omitempty"`
	DeviceID       string `json:"device_id"`
	TeamID         string `json:"team_id,omitempty"`
	ProxyURL       string `json:"proxy_url,omitempty"`
}

type accountFile struct { Accounts []storedCredential `json:"accounts"` }

func (s storedCredential) runtime() account.Credential {
	return account.Credential{
		ID:s.ID,Label:s.Label,Token:s.Token,RefreshToken:s.RefreshToken,TokenExpiresAt:s.TokenExpiresAt,
		DeviceID:s.DeviceID,TeamID:s.TeamID,ProxyURL:s.ProxyURL,
	}
}

func Load() (Runtime,error) { return load(os.Getenv,os.ReadFile) }

func load(getenv func(string)string, readFile func(string)([]byte,error)) (Runtime,error) {
	r:=Runtime{
		Listen: strings.TrimSpace(getenv("VERDENT_LISTEN")),
		APIKey: getenv("VERDENT_API_KEY"),
		AdminUser: strings.TrimSpace(getenv("VERDENT_ADMIN_USER")),
		AdminPassword: getenv("VERDENT_ADMIN_PASSWORD"),
		AccountsFile: strings.TrimSpace(getenv("VERDENT_ACCOUNTS_FILE")),
		PublicBaseURL: strings.TrimSpace(getenv("VERDENT_PUBLIC_BASE_URL")),
		AuthBaseURL: strings.TrimSpace(getenv("VERDENT_AUTH_BASE_URL")),
		LoginBaseURL: strings.TrimSpace(getenv("VERDENT_LOGIN_BASE_URL")),
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
	if r.AdminUser=="" { r.AdminUser="admin" }
	if r.AdminPassword=="" { r.AdminPassword=r.APIKey }
	if r.AccountsFile=="" { r.AccountsFile="data/accounts.json" }
	r.RequestTimeout=5*time.Minute
	if raw:=strings.TrimSpace(getenv("VERDENT_REQUEST_TIMEOUT"));raw!="" { d,err:=time.ParseDuration(raw);if err!=nil{return Runtime{},fmt.Errorf("VERDENT_REQUEST_TIMEOUT: %w",err)};if d<=0{return Runtime{},errors.New("VERDENT_REQUEST_TIMEOUT must be positive")};r.RequestTimeout=d }
	if r.Protocol.AppVersion=="" { return Runtime{},errors.New("VERDENT_APP_VERSION is required") }
	if r.Protocol.BetaHeader=="" { return Runtime{},errors.New("VERDENT_PROXY_BETA is required") }
	if r.Protocol.Sign=="" { return Runtime{},errors.New("VERDENT_PROXY_SIGN is required") }

	if data,err:=readFile(r.AccountsFile);err==nil {
		var stored []storedCredential
		var wrapped accountFile
		if err:=json.Unmarshal(data,&wrapped);err==nil && len(wrapped.Accounts)>0 { stored=wrapped.Accounts } else {
			if err:=json.Unmarshal(data,&stored);err!=nil{return Runtime{},fmt.Errorf("decode accounts file: %w",err)}
		}
		for _,item:=range stored { r.Accounts=append(r.Accounts,item.runtime()) }
	} else if !errors.Is(err,os.ErrNotExist) {
		return Runtime{},fmt.Errorf("read accounts file: %w",err)
	}
	if len(r.Accounts)==0 {
		if token:=strings.TrimSpace(getenv("VERDENT_TOKEN"));token!="" {
			id:=strings.TrimSpace(getenv("VERDENT_ACCOUNT_ID"));if id==""{id=account.StableAccountID(token,strings.TrimSpace(getenv("VERDENT_TEAM_ID")))}
			deviceID:=strings.TrimSpace(getenv("VERDENT_DEVICE_ID"));if deviceID==""{deviceID=account.NewDeviceID()}
			r.Accounts=[]account.Credential{{
				ID:id,Label:strings.TrimSpace(getenv("VERDENT_ACCOUNT_LABEL")),Token:token,
				DeviceID:deviceID,TeamID:strings.TrimSpace(getenv("VERDENT_TEAM_ID")),
				ProxyURL:strings.TrimSpace(getenv("VERDENT_PROXY")),
			}}
		}
	}
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
