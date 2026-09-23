package account

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreRoundTripKeepsSecretOffRuntimeJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store := FileStore{Path: path}
	want := []Credential{{ID:"a",Label:"one",Token:"secret",RefreshToken:"refresh-secret",TokenExpiresAt:1900000000,DeviceID:"dev",TeamID:"0",ProxyURL:"http://127.0.0.1:8080",Disabled:true,Suspended:true,SuspensionError:"80006 suspended"}}
	if err := store.Save(want); err != nil { t.Fatal(err) }
	info, err := os.Stat(path); if err != nil { t.Fatal(err) }
	if info.Mode().Perm() != 0o600 { t.Fatalf("mode=%o", info.Mode().Perm()) }
	got, err := store.Load(); if err != nil { t.Fatal(err) }
	if len(got) != 1 || got[0].Token != "secret" || got[0].RefreshToken != "refresh-secret" || got[0].TokenExpiresAt != 1900000000 || got[0].ProxyURL != want[0].ProxyURL || !got[0].Disabled || !got[0].Suspended || got[0].SuspensionError!="80006 suspended" { t.Fatalf("bad round trip: %#v", got) }
}

func TestTokenMetadataAndStableID(t *testing.T) {
	token := "header.eyJ1c2VyX2lkIjoxMjM0NTY3ODkwMTIzNDU2Nzg5LCJlbWFpbCI6Im1lQGV4YW1wbGUuY29tIiwiZXhwIjoyMDAwMDAwMDAwfQ.signature"
	meta := MetadataFromToken(token)
	if meta.UID != "1234567890123456789" || meta.Email != "me@example.com" || meta.Expires != 2000000000 {
		t.Fatalf("bad metadata: %#v", meta)
	}
	if StableAccountID(token,"0") != StableAccountID(token,"0") { t.Fatal("account id must be stable") }
	if StableAccountIDForIdentity("1234567890123456789","0") != StableAccountIDForIdentity("1234567890123456789","0") { t.Fatal("identity-based account id must be stable") }
	if NewDeviceID() == "" { t.Fatal("device id is empty") }
}


func TestFileStoreOldAccountsDefaultToEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err:=os.WriteFile(path,[]byte(`{"accounts":[{"id":"a","token":"secret","device_id":"dev"}]}`),0o600);err!=nil{t.Fatal(err)}
	got,err:=FileStore{Path:path}.Load();if err!=nil{t.Fatal(err)}
	if len(got)!=1 || got[0].Disabled { t.Fatalf("legacy account should default enabled: %#v",got) }
}
