package rankings

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "time"

 "github.com/jackc/pgx/v5"
)

type Model struct {
 ModelID string `json:"model_id"`
 DisplayName string `json:"display_name"`
 Calls string `json:"calls"`
 Errors string `json:"errors"`
 CreditsUnits string `json:"credits_units"`
}
type PublicBadge struct {
 Code string `json:"code"`
 Name string `json:"name"`
 IconPath string `json:"icon_path"`
 Description string `json:"description"`
}
type Entry struct {
 Badges []PublicBadge `json:"badges"`
 Rank int64 `json:"rank,string"`
 DisplayName string `json:"display_name"`
 AvatarID string `json:"avatar_id"`
 Value string `json:"value"`
 Calls string `json:"calls"`
 Errors string `json:"errors"`
 CreditsUnits string `json:"credits_units"`
 Models []Model `json:"models"`
}
type Page struct {
 State string `json:"state"`
 Metric string `json:"metric"`
 Period string `json:"period"`
 PeriodStart time.Time `json:"period_start"`
 PeriodEnd *time.Time `json:"period_end"`
 LastUpdated *time.Time `json:"last_updated"`
 Items []Entry `json:"items"`
 Total int64 `json:"total,string"`
 Page int `json:"page"`
 PageSize int `json:"page_size"`
 HistoricalPeriods []string `json:"historical_periods"`
 MyRank *Entry `json:"my_rank,omitempty"`
}
func metricDomain(metric string) string {
 if metric=="TOTAL_ASSETS"{return "ASSETS"};if strings.HasPrefix(metric,"RP_"){return "RP"};return "GAMES"
}
const orderedEntries = `WITH ordered AS (
 SELECT rank() OVER(ORDER BY e.value DESC) ranking,e.newapi_user_id,p.display_name,p.avatar_id,
  e.value::text,e.calls::text,e.errors::text,e.credits_units::text,e.models,
 EXISTS(SELECT 1 FROM identity.account_badges b WHERE b.newapi_user_id=e.newapi_user_id AND b.badge_code='gambler-ruler-202609') has_gambler_badge
 FROM rankings.entries e JOIN identity.master_profiles p USING(newapi_user_id)
 WHERE e.snapshot_id=$1::uuid AND e.metric=$2 AND e.model_id=$3
  AND e.model_scope=CASE WHEN $3='' THEN 'ALL' ELSE 'MODEL' END
) `

func scanEntry(row pgx.Row) (Entry,error) {
 var entry Entry;var raw []byte;var gambler bool
 err:=row.Scan(&entry.Rank,&entry.DisplayName,&entry.AvatarID,&entry.Value,&entry.Calls,&entry.Errors,&entry.CreditsUnits,&raw,&gambler)
 if err==nil {err=json.Unmarshal(raw,&entry.Models)}
 if entry.Models==nil{entry.Models=[]Model{}}
 entry.Badges=[]PublicBadge{}
 if gambler {entry.Badges=append(entry.Badges,PublicBadge{"gambler-ruler-202609","赌怪","ui/badges/gambler-ruler.e0c69ec268a86ee0.png","纪念资产调整前总资产超过十亿的御主"})}
 return entry,err
}

// ownUser comes from the verified session on the private /me endpoint. Public
// readers always pass zero; user IDs do not appear in serialized entries.
func (s *Service) Read(ctx context.Context,q Query,ownUser int64) (Page,error) {
 result:=Page{State:"UNAVAILABLE",Metric:q.Metric,Period:q.Period,Items:[]Entry{},HistoricalPeriods:[]string{},Page:q.Page,PageSize:50}
 if !q.valid()||ownUser<0{return result,ErrQuery}
 now:=time.Now().UTC();if !s.enabled(now){return result,nil}
 start,end,err:=periodBounds(q.Period,q.Date,now,s.activation);if err!=nil{return result,err}
 result.PeriodStart,result.PeriodEnd=start,end
 err=s.store.WithTx(ctx,func(tx pgx.Tx)error{
  if _,e:=tx.Exec(ctx,`SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY`);e!=nil{return e}
  var id string;var built,checked time.Time
  e:=tx.QueryRow(ctx,`SELECT s.snapshot_id::text,s.built_at,s.source_checked_at
   FROM rankings.published_pointers p JOIN rankings.snapshots s USING(snapshot_id)
   WHERE p.domain=$1 AND p.metric=$2 AND p.period=$3 AND p.period_start=$4 AND p.activation_at=$5
    AND s.status='READY'`,metricDomain(q.Metric),q.Metric,q.Period,start,s.activation).Scan(&id,&built,&checked)
  if errors.Is(e,pgx.ErrNoRows){return nil};if e!=nil{return e}
  result.State="READY";result.LastUpdated=&checked
  if (end==nil||end.After(now))&&(now.Sub(built)>currentSnapshotMaxAge||now.Sub(checked)>currentSnapshotMaxAge){result.State="STALE"}
  e=tx.QueryRow(ctx,`SELECT count(*) FROM rankings.entries WHERE snapshot_id=$1::uuid AND metric=$2 AND model_id=$3 AND model_scope=CASE WHEN $3='' THEN 'ALL' ELSE 'MODEL' END`,id,q.Metric,q.Model).Scan(&result.Total);if e!=nil{return e}
  rows,e:=tx.Query(ctx,orderedEntries+`SELECT ranking,display_name,avatar_id,value,calls,errors,credits_units,models,has_gambler_badge FROM ordered ORDER BY ranking,newapi_user_id LIMIT 50 OFFSET $4`,id,q.Metric,q.Model,(q.Page-1)*50);if e!=nil{return e}
  for rows.Next(){row,scanErr:=scanEntry(rows);if scanErr!=nil{rows.Close();return scanErr};result.Items=append(result.Items,row)}
  e=rows.Err();rows.Close();if e!=nil{return e}
  if ownUser>0 {
   row,readErr:=scanEntry(tx.QueryRow(ctx,orderedEntries+`SELECT ranking,display_name,avatar_id,value,calls,errors,credits_units,models,has_gambler_badge FROM ordered WHERE newapi_user_id=$4`,id,q.Metric,q.Model,ownUser))
   if readErr==nil{result.MyRank=&row}else if !errors.Is(readErr,pgx.ErrNoRows){return readErr}
  }
  if q.Period=="DAY"||q.Period=="WEEK" {
    periods,readErr:=tx.Query(ctx,`SELECT DISTINCT to_char(period_start AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD')
     FROM rankings.published_pointers WHERE domain=$1 AND metric=$2 AND period=$3 AND activation_at=$4
     ORDER BY 1 DESC`,metricDomain(q.Metric),q.Metric,q.Period,s.activation);if readErr!=nil{return readErr}
   for periods.Next(){var date string;if e=periods.Scan(&date);e!=nil{periods.Close();return e};result.HistoricalPeriods=append(result.HistoricalPeriods,date)}
   e=periods.Err();periods.Close()
  }
  return e
 });if err!=nil{return Page{},err};return result,nil
}
