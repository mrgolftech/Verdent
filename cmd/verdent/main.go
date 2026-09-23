package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mrgolftech/Verdent/internal/account"
	"github.com/mrgolftech/Verdent/internal/apikey"
	"github.com/mrgolftech/Verdent/internal/config"
	"github.com/mrgolftech/Verdent/internal/server"
	"github.com/mrgolftech/Verdent/internal/verdentauth"
)

func main() {
	cfg,err:=config.Load();if err!=nil{log.Fatalf("configuration error: %v",err)}
	router:=account.NewRouter(cfg.Accounts)
	api:=server.New(router,cfg.Protocol)
	api.APIKey=cfg.APIKey
	api.AdminUser=cfg.AdminUser
	api.AdminPassword=cfg.AdminPassword
	api.PublicBaseURL=cfg.PublicBaseURL
	if cfg.Version!="" { api.Version=cfg.Version }
	api.AccountStore=account.FileStore{Path:cfg.AccountsFile}
	keyManager,err:=apikey.NewManager(apikey.FileStore{Path:cfg.KeysFile});if err!=nil{log.Fatalf("keystore error: %v",err)}
	api.Keys=keyManager
	api.RequestTimeout=cfg.RequestTimeout
	api.OAuth=verdentauth.New(verdentauth.Config{
		AuthBaseURL:cfg.AuthBaseURL,
		LoginBaseURL:cfg.LoginBaseURL,
		UserAgent:cfg.Protocol.UserAgent,
	},nil)

	httpServer:=&http.Server{Addr:cfg.Listen,Handler:api.Handler(),ReadHeaderTimeout:10*time.Second,IdleTimeout:2*time.Minute}
	ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stop()
	errCh:=make(chan error,1)
	go func(){ log.Printf("Verdent gateway listening on %s with %d account(s)",cfg.Listen,len(cfg.Accounts));errCh<-httpServer.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
		if err:=httpServer.Shutdown(shutdownCtx);err!=nil{log.Printf("shutdown error: %v",err)}
	case err:=<-errCh:
		if err!=nil && !errors.Is(err,http.ErrServerClosed){log.Fatalf("server error: %v",err)}
	}
}
