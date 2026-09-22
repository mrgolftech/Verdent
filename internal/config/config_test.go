package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSingleAccountFromEnv(t *testing.T) {
	env:=map[string]string{
		"VERDENT_APP_VERSION":"2.test","VERDENT_PROXY_BETA":"beta","VERDENT_PROXY_SIGN":"protocol-sign-for-test-only",
		"VERDENT_TOKEN":"token","VERDENT_DEVICE_ID":"dev","VERDENT_PROXY":"http://127.0.0.1:8080",
	}
	r,err:=load(func(k string)string{return env[k]},os.ReadFile);if err!=nil{t.Fatal(err)}
	if r.Listen!=":5084" || len(r.Accounts)!=1 || r.Accounts[0].ProxyURL!="http://127.0.0.1:8080" { t.Fatalf("bad runtime: %#v",r) }
}

func TestLoadAccountsFile(t *testing.T) {
	dir:=t.TempDir();path:=filepath.Join(dir,"accounts.json")
	data:=`{"accounts":[{"id":"a","token":"ta","device_id":"da"},{"id":"b","token":"tb","device_id":"db","proxy_url":"https://127.0.0.1:8443"}]}`
	if err:=os.WriteFile(path,[]byte(data),0600);err!=nil{t.Fatal(err)}
	env:=map[string]string{"VERDENT_APP_VERSION":"2.test","VERDENT_PROXY_BETA":"beta","VERDENT_PROXY_SIGN":"protocol-sign-for-test-only","VERDENT_ACCOUNTS_FILE":path}
	r,err:=load(func(k string)string{return env[k]},os.ReadFile);if err!=nil{t.Fatal(err)}
	if len(r.Accounts)!=2 || r.Accounts[0].TeamID!="0" || r.Accounts[1].ProxyURL=="" { t.Fatalf("bad accounts: %#v",r.Accounts) }
}

func TestLoadRejectsMissingProtocolConfig(t *testing.T) {
	env:=map[string]string{"VERDENT_TOKEN":"t","VERDENT_DEVICE_ID":"d"}
	if _,err:=load(func(k string)string{return env[k]},os.ReadFile);err==nil { t.Fatal("expected missing protocol config error") }
}
