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

func TestLoadVersionEnvOverride(t *testing.T) {
	base:=map[string]string{"VERDENT_APP_VERSION":"2.test","VERDENT_PROXY_BETA":"beta","VERDENT_PROXY_SIGN":"protocol-sign-for-test-only"}
	r,err:=load(func(k string)string{return base[k]},os.ReadFile);if err!=nil{t.Fatal(err)}
	if r.Version!="" { t.Fatalf("unset VERDENT_VERSION should leave the build default untouched, got %q",r.Version) }
	base["VERDENT_VERSION"]=" v0.1.0-alpha.4 "
	r,err=load(func(k string)string{return base[k]},os.ReadFile);if err!=nil{t.Fatal(err)}
	if r.Version!="v0.1.0-alpha.4" { t.Fatalf("version=%q",r.Version) }
}


func TestLoadRejectsMissingProtocolConfig(t *testing.T) {
	env:=map[string]string{"VERDENT_TOKEN":"t","VERDENT_DEVICE_ID":"d"}
	if _,err:=load(func(k string)string{return env[k]},os.ReadFile);err==nil { t.Fatal("expected missing protocol config error") }
}


func TestLoadCapturedDesktopTemplateAndProtocolDefaults(t *testing.T) {
	dir:=t.TempDir()
	templatePath:=filepath.Join(dir,"template.json")
	template:=`{"channel":"deck","system":"captured-ciphertext","thinking":{"type":"enabled","budget_tokens":4000},"agent_name":"VerdentDeck","encrypt":true,"is_eco":false,"is_auto":false,"is_free":false,"is_limit_free":false,"native_api":false,"effort":"high","max_tokens":64000,"temperature":1,"model_catalog_version":"model-catalog-live","custom_trace_tags_tmp":[],"env":{"platform":"win32","os_version":"Windows_NT 10.0.26200","shell":"gitbash","today_date":"2026-09-22"}}`
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
	if r.Protocol.Channel!="deck" || r.Protocol.AgentName!="VerdentDeck" { t.Fatalf("identity fields not loaded: %#v",r.Protocol) }
	if r.Protocol.Thinking==nil || r.Protocol.Thinking.Type!="enabled" || r.Protocol.Thinking.BudgetTokens!=4000 { t.Fatalf("thinking=%#v",r.Protocol.Thinking) }
	if r.Protocol.Effort!="high" || r.Protocol.MaxTokens!=64000 || r.Protocol.Temperature==nil || *r.Protocol.Temperature!=1 { t.Fatalf("request defaults not loaded: %#v",r.Protocol) }
	if r.Protocol.Environment.Platform!="win32" || r.Protocol.Environment.OSVersion!="Windows_NT 10.0.26200" || r.Protocol.Environment.Shell!="gitbash" { t.Fatalf("env=%#v",r.Protocol.Environment) }
	if r.Protocol.OSType!="windows" { t.Fatalf("X-OS-Type should follow captured win32 platform, got %q",r.Protocol.OSType) }
	if r.Protocol.TraceTags==nil || len(r.Protocol.TraceTags)!=0 { t.Fatalf("trace tags=%#v",r.Protocol.TraceTags) }
	if r.Protocol.NativeAPI || r.Protocol.IsEco || r.Protocol.IsAuto || r.Protocol.IsFree || r.Protocol.IsLimitFree { t.Fatal("2.15.1 boolean defaults should remain false") }
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


func TestDesktopHeaderIdentityOverrides(t *testing.T) {
	env:=map[string]string{
		"VERDENT_APP_VERSION":"2.15.1","VERDENT_PROXY_BETA":"hybrid-stream@20250919","VERDENT_PROXY_SIGN":"protocol-sign-for-test-only",
		"VERDENT_OS_TYPE":"windows","VERDENT_OS_NAME":"Windows_NT 10.0.26200","VERDENT_CPU_ARCH":"x64",
		"VERDENT_DEVICE_TYPE":"pc","VERDENT_DEVICE_MODEL":"AMD Ryzen Test",
	}
	r,err:=load(func(k string)string{return env[k]},os.ReadFile);if err!=nil{t.Fatal(err)}
	if r.Protocol.OSType!="windows" || r.Protocol.OSName!="Windows_NT 10.0.26200" || r.Protocol.CPUArch!="x64" || r.Protocol.DeviceType!="pc" || r.Protocol.DeviceModel!="AMD Ryzen Test" {
		t.Fatalf("desktop header identity not configurable: %#v",r.Protocol)
	}
}
