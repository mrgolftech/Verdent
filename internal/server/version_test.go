package server

import (
	"testing"

	"github.com/mrgolftech/Verdent/internal/account"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

func TestNewUsesBuildVersion(t *testing.T) {
	previous:=Version
	Version="v0.1.0-alpha.5"
	defer func(){ Version=previous }()

	s:=New(account.NewRouter(nil),protocol.Config{})
	if s.Version!="v0.1.0-alpha.5" {
		t.Fatalf("version=%q",s.Version)
	}
}
