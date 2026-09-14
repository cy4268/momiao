package main

import (
 "errors"
 "path/filepath"
 "strconv"

 "github.com/cy4268/momiao/internal/platform"
)

func loadRefillConfig(cfg *config,lookup func(string)(string,bool)) error {
 names:=[]string{"MOMIAO_REFILL_SOCKET","MOMIAO_REFILL_KEY_FILE","MOMIAO_REFILL_LOW","MOMIAO_REFILL_TARGET","MOMIAO_REFILL_MAX"}
 values:=make([]string,len(names));count:=0
 for i,name:=range names {value,set:=lookup(name);if set{count++;values[i]=value}}
 if count==0{return nil}
 invalid:=errors.New("active quota refill requires a separate absolute Unix socket, private key file, Native quota port, and explicit 0 <= LOW < TARGET <= MAX raw quota values")
 if count!=len(names)||cfg.NativeQuotaKeyFile==""||cfg.WalletDSNFile==""||!filepath.IsAbs(values[0])||!filepath.IsAbs(values[1]){return invalid}
 socket,key:=filepath.Clean(values[0]),filepath.Clean(values[1])
 if socket==cfg.ListenSocket||socket==cfg.NewAPISocket||socket==key{return invalid}
 numbers:=make([]int64,3)
 for i,value:=range values[2:] {number,err:=strconv.ParseInt(value,10,32);if err!=nil||number<0||strconv.FormatInt(number,10)!=value{return invalid};numbers[i]=number}
 if numbers[0]>=numbers[1]||numbers[1]>numbers[2]{return invalid}
 cfg.RefillSocket,cfg.RefillKeyFile=socket,key
 cfg.RefillPolicy=platform.ActiveQuotaRefillPolicy{Enabled:true,LowWatermark:numbers[0],TargetWatermark:numbers[1],MaxActiveBuffer:numbers[2]}
 return nil
}
