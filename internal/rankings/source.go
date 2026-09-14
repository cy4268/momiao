package rankings

import (
 "context"
 "errors"
 "time"

 "github.com/cy4268/momiao/internal/platform"
)

var ErrUnavailable = errors.New("rankings unavailable")
var ErrQuery = errors.New("invalid ranking query")

type AssetReader interface {
 ReadRankingAssets(context.Context, int64) (string, time.Time, error)
}

type Service struct {
 store *platform.Store
 activation time.Time
 assets AssetReader
 sourceID string
 attributionHealth func(context.Context) (time.Time,error)
}
type Options struct {
 ActivationTime time.Time
 SourceInstanceID string
 Assets AssetReader
 AttributionHealth func(context.Context) (time.Time,error)
}

// Zero activation deliberately keeps public aggregation off. No historical log
// inference or deployment timestamp is substituted for the cutover decision.
func NewService(store *platform.Store, options Options) *Service {
 return &Service{store:store,activation:options.ActivationTime.UTC(),assets:options.Assets,sourceID:options.SourceInstanceID,attributionHealth:options.AttributionHealth}
}

type Query struct {
 Metric string
 Period string
 Date string
 Model string
 Page int
}
func (q Query) valid() bool {
 switch q.Metric {case "TOTAL_ASSETS","GAME_PROFIT","BIGGEST_WIN","TOTAL_WAGERED","POKER_PROFIT","RP_CALLS","RP_ERRORS","RP_CREDITS":default:return false}
 if q.Page<1 || q.Page>10000 || len(q.Model)>256 {return false}
 if q.Model!="" && q.Metric!="RP_CALLS" && q.Metric!="RP_ERRORS" && q.Metric!="RP_CREDITS" {return false}
 if q.Metric=="TOTAL_ASSETS" {return q.Period=="CURRENT"&&q.Date==""}
 return q.Period=="DAY"||q.Period=="WEEK"||q.Period=="ALL_TIME"&&q.Date==""
}

func periodBounds(period, date string, now, activation time.Time) (time.Time,*time.Time,error) {
 zone:=time.FixedZone("Asia/Shanghai",8*60*60)
 if period=="ALL_TIME"||period=="CURRENT" {return activation,nil,nil}
 anchor:=now.In(zone)
 if date!="" {var err error;anchor,err=time.ParseInLocation("2006-01-02",date,zone);if err!=nil{return time.Time{},nil,ErrQuery}}
 start:=time.Date(anchor.Year(),anchor.Month(),anchor.Day(),0,0,0,0,zone)
 days:=1
 if period=="WEEK" {start=start.AddDate(0,0,-(int(start.Weekday())+6)%7);days=7} else if period!="DAY" {return time.Time{},nil,ErrQuery}
 end:=start.AddDate(0,0,days)
 if start.After(now)||!end.After(activation) {return time.Time{},nil,ErrQuery}
 return start.UTC(),&end,nil
}
