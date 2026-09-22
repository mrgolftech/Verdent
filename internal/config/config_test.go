package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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


func TestLoadCapturedDesktopTemplateAndProtocolDefaults(t *testing.T) {
	dir:=t.TempDir()
	templatePath:=filepath.Join(dir,"template.json")
	template:=`{"system":"captured-ciphertext","model_catalog_version":"model-catalog-live","native_api":false}`
	if err:=os.WriteFile(templatePath,[]byte(template),0600);err!=nil{t.Fatal(err)}
	env:=map[string]string{
		"VERDENT_APP_VERSION":"2.15.1",
		"VERDENT_PROXY_BETA":"hybrid-stream@20250919",
		"VERDENT_PROXY_SIGN":"protocol-sign-for-test-only",
		"VERDENT_SYSTEM_TEMPLATE_FILE":templatePath,
	}
	r,err:=load(func(k string)string{return env[k]},os.ReadFile);if err!=nil{t.Fatal(err)}
	if r.Protocol.SystemCiphertext!="captured-ciphertext" || r.Protocol.ModelCatalogVersion!="model-catalog-live" {
		t.Fatalf("template not loaded: %#v",r.Protocol)
	}
	if r.Protocol.NativeAPI { t.Fatal("native_api should match current Desktop capture") }
	if r.Protocol.MinRequestInterval!=1200*time.Millisecond { t.Fatalf("interval=%v",r.Protocol.MinRequestInterval) }
	if len(r.Protocol.RetryDelays)!=3 || r.Protocol.RetryDelays[0]!=10*time.Second || r.Protocol.RetryDelays[2]!=45*time.Second {
		t.Fatalf("retry delays=%v",r.Protocol.RetryDelays)
	}
}

func TestNativeAPIEnvOverridesTemplate(t *testing.T) {
	dir:=t.TempDir()
	templatePath:=filepath.Join(dir,"template.json")
	if err:=os.WriteFile(templatePath,[]byte(`{"system":"captured","native_api":false}`),0600);err!=nil{t.Fatal(err)}
	env:=map[string]string{
		"VERDENT_APP_VERSION":"2.15.1","VERDENT_PROXY_BETA":"beta","VERDENT_PROXY_SIGN":"protocol-sign-for-test-only",
		"VERDENT_SYSTEM_TEMPLATE_FILE":templatePath,"VERDENT_NATIVE_API":"true",
	}
	r,err:=load(func(k string)string{return env[k]},os.ReadFile);if err!=nil{t.Fatal(err)}
	if !r.Protocol.NativeAPI { t.Fatal("explicit VERDENT_NATIVE_API should win") }
}
