package server

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) handleModels(w http.ResponseWriter,r *http.Request) {
	a,err:=s.selectAccount(r)
	if err!=nil { writeAPIError(w,http.StatusServiceUnavailable,"no_account",err.Error()); return }
	client,err:=s.NewClient(*a,s.ProtocolConfig,s.RequestTimeout)
	if err!=nil { writeAPIError(w,http.StatusInternalServerError,"client_config",err.Error()); return }
	ctx:=r.Context()
	if s.RequestTimeout>0 { var cancel context.CancelFunc; ctx,cancel=context.WithTimeout(ctx,s.RequestTimeout); defer cancel() }
	models,err:=client.DiscoverModels(ctx,a.Credential.Token)
	if err!=nil { writeAPIError(w,http.StatusBadGateway,"model_catalog",err.Error()); return }
	created:=time.Now().Unix()
	data:=make([]map[string]any,0,len(models))
	for _,m:=range models {
		data=append(data,map[string]any{"id":m.ID,"object":"model","created":created,"owned_by":"verdent"})
	}
	writeJSON(w,http.StatusOK,map[string]any{"object":"list","data":data})
}
