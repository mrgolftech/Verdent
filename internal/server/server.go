package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mrgolftech/Verdent/internal/account"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

type Server struct {
	Accounts       *account.Router
	ProtocolConfig protocol.Config
	RequestTimeout time.Duration
	APIKey         string
	NewClient      func(account.Account, protocol.Config, time.Duration) (*protocol.Client,error)
}

func New(accounts *account.Router,cfg protocol.Config) *Server {
	return &Server{Accounts:accounts,ProtocolConfig:cfg,RequestTimeout:5*time.Minute,NewClient:account.NewProtocolClient}
}

func (s *Server) Handler() http.Handler {
	mux:=http.NewServeMux()
	mux.HandleFunc("GET /healthz",func(w http.ResponseWriter,r *http.Request){ writeJSON(w,http.StatusOK,map[string]any{"ok":true}) })
	mux.HandleFunc("GET /v1/models",s.auth(s.handleModels))
	mux.HandleFunc("POST /v1/chat/completions",s.auth(s.handleChatCompletions))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter,r *http.Request){
		if s.APIKey!="" {
			got:=strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer "))
			if got!=s.APIKey { writeAPIError(w,http.StatusUnauthorized,"invalid_api_key","Invalid API key"); return }
		}
		next(w,r)
	}
}

func (s *Server) selectAccount(r *http.Request) (*account.Account,error) {
	if s.Accounts==nil { return nil,fmt.Errorf("no account router configured") }
	session:=r.Header.Get("X-Verdent-Session-ID")
	explicit:=r.Header.Get("X-Verdent-Account")
	a:=s.Accounts.Select(session,explicit)
	if a==nil { return nil,fmt.Errorf("no eligible Verdent account") }
	return a,nil
}

func randomID(prefix string) string {
	b:=make([]byte,16)
	if _,err:=rand.Read(b);err!=nil { return fmt.Sprintf("%s%d",prefix,time.Now().UnixNano()) }
	return prefix+hex.EncodeToString(b)
}

func requestIDs() protocol.RequestIDs {
	return protocol.RequestIDs{SessionID:randomID("session_"),ConvID:randomID("conv_"),ReactID:randomID("model_agent_")}
}

func writeJSON(w http.ResponseWriter,status int,v any) {
	w.Header().Set("Content-Type","application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter,status int,code,message string) {
	writeJSON(w,status,map[string]any{"error":map[string]any{"message":message,"type":"invalid_request_error","code":code}})
}
