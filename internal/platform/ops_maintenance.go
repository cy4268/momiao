package platform

import (
 "bytes"
 "context"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "slices"
 "strconv"
 "time"

 "github.com/jackc/pgx/v5"
)

var maintenanceScopes=[]string{"CHALDEA_USER_WRITES","WALLET_EXCHANGE","REWARDS","DIRECT_PLAY_NEW_ROUNDS","POKER_NEW_TABLES_NEW_HANDS","RANKINGS_PUBLISHING","ANNOUNCEMENTS_SCHEDULING"}
type MaintenanceWindow struct {
 ID string `json:"maintenance_id"`
 State string `json:"state"`
 Reason string `json:"reason"`
 Environment string `json:"environment"`
 Version int64 `json:"version,string"`
 Scopes []string `json:"scopes"`
 ScheduledStart *time.Time `json:"scheduled_start_at"`
 ScheduledEnd *time.Time `json:"scheduled_end_at"`
 EstimatedEnd *time.Time `json:"estimated_end_at"`
 ActivatedAt *time.Time `json:"activated_at"`
 EndedAt *time.Time `json:"ended_at"`
 OperationID string `json:"operation_id"`
 AnnouncementID *string `json:"announcement_id"`
 CriticalNoticeID *string `json:"critical_notice_id"`
}
type maintenanceInput struct {
 Scopes []string `json:"scopes,omitempty"`
 Start *time.Time `json:"scheduled_start_at,omitempty"`
 End *time.Time `json:"scheduled_end_at,omitempty"`
 EstimatedEnd *time.Time `json:"estimated_end_at,omitempty"`
 AnnouncementID string `json:"announcement_id,omitempty"`
 CriticalNoticeID string `json:"critical_notice_id,omitempty"`
}
type maintenanceOperation struct{ action string;critical bool }
var ErrMaintenanceOverlap=errors.New("MAINTENANCE_SCOPE_OVERLAP")
func criticalMaintenance(scopes []string)bool{return slices.Contains(scopes,"CHALDEA_USER_WRITES")||slices.Contains(scopes,"WALLET_EXCHANGE")}

func MaintenanceOpsBindings() []OpsOperationBinding {
 result:=[]OpsOperationBinding{}
 for _,action:=range []string{"START","SCHEDULE","END","CANCEL"}{
  for _,critical:=range []bool{false,true}{
  operationType,risk,confirmation:="MAINTENANCE_"+action,OpsRiskImpactful,OpsConfirmExplicit
  if critical{operationType+="_CRITICAL";risk=OpsRiskCritical;confirmation=OpsConfirmTyped}
  result=append(result,OpsOperationBinding{Descriptor:OpsOperationDescriptor{
   OperationType:operationType,Risk:risk,RequiredPermission:"maintenance.write",AllowedRoles:[]string{"SUPER_ADMIN"},
   TargetType:"MAINTENANCE",InputSchemaVersion:"1",ImpactSchemaVersion:"1",RequiresReason:true,RequiresFreshAuth:true,ConfirmationMode:confirmation,
   SupportsSchedule:action=="SCHEDULE",ExecutionMode:OpsSameDatabase,
  },Handler:maintenanceOperation{action:action,critical:critical}})
  }
 }
 return result
}
func decodeMaintenanceInput(raw json.RawMessage)(maintenanceInput,error){
 var input maintenanceInput
 decoder:=json.NewDecoder(bytes.NewReader(raw));decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&input);err!=nil{return input,ErrOpsInvalid}
 if err:=decoder.Decode(new(any));err!=io.EOF{return input,ErrOpsInvalid}
 return input,nil
}
func (h maintenanceOperation) Canonicalize(raw json.RawMessage)(json.RawMessage,error){
 input,err:=decodeMaintenanceInput(raw);if err!=nil{return nil,err}
 if h.action=="START"||h.action=="SCHEDULE" {
  if len(input.Scopes)==0||len(input.Scopes)>len(maintenanceScopes){return nil,ErrOpsInvalid}
  slices.Sort(input.Scopes)
  for i,scope:=range input.Scopes{if !slices.Contains(maintenanceScopes,scope)||i>0&&input.Scopes[i-1]==scope{return nil,ErrOpsInvalid}}
  if h.action=="SCHEDULE"&&(input.Start==nil||input.End!=nil&&!input.End.After(*input.Start)){return nil,ErrOpsInvalid}
  if h.action=="START"&&input.Start!=nil{return nil,ErrOpsInvalid}
 }else if len(input.Scopes)!=0||input.Start!=nil||input.End!=nil||input.EstimatedEnd!=nil||input.AnnouncementID!=""||input.CriticalNoticeID!=""{return nil,ErrOpsInvalid}
 if input.AnnouncementID!=""&&!ValidOperationKey(input.AnnouncementID)||input.CriticalNoticeID!=""&&!ValidOperationKey(input.CriticalNoticeID){return nil,ErrOpsInvalid}
 if input.Start!=nil{value:=input.Start.UTC();input.Start=&value};if input.End!=nil{value:=input.End.UTC();input.End=&value}
 if input.EstimatedEnd!=nil{value:=input.EstimatedEnd.UTC();input.EstimatedEnd=&value}
 return json.Marshal(input)
}
const maintenanceColumns=`w.maintenance_id::text,w.state,w.reason,w.environment,w.state_version,
 ARRAY(SELECT scope FROM ops.maintenance_window_scopes WHERE maintenance_id=w.maintenance_id ORDER BY scope),
 w.scheduled_start_at,w.scheduled_end_at,w.estimated_end_at,w.activated_at,w.ended_at,w.operation_id::text,w.announcement_id::text,
 (SELECT critical_notice_id::text FROM ops.critical_notice_state n WHERE n.maintenance_id=w.maintenance_id)`
func scanMaintenance(row pgx.Row)(MaintenanceWindow,error){var result MaintenanceWindow;err:=row.Scan(&result.ID,&result.State,&result.Reason,&result.Environment,&result.Version,&result.Scopes,&result.ScheduledStart,&result.ScheduledEnd,&result.EstimatedEnd,&result.ActivatedAt,&result.EndedAt,&result.OperationID,&result.AnnouncementID,&result.CriticalNoticeID);return result,err}
func lockMaintenanceGuards(ctx context.Context,tx pgx.Tx,scopes []string)error{
 rows,err:=tx.Query(ctx,`SELECT scope FROM ops.maintenance_scope_guards WHERE scope=ANY($1::text[]) ORDER BY scope FOR UPDATE`,scopes)
 if err!=nil{return err};defer rows.Close();count:=0
 for rows.Next(){count++};if err=rows.Err();err!=nil{return err};if count!=len(scopes){return ErrOpsInvalid};return nil
}
func maintenanceOverlap(ctx context.Context,tx pgx.Tx,scopes []string,except string,start time.Time,end *time.Time)(bool,error){
 var conflict bool
 err:=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM ops.maintenance_windows w
 JOIN ops.maintenance_window_scopes s USING(maintenance_id)
 WHERE s.scope=ANY($1::text[]) AND w.maintenance_id<>$2::uuid
 AND w.state IN('DRAFT','SCHEDULED','ACTIVE','ENDING','ENDING_FAILED')
 AND tstzrange(coalesce(w.scheduled_start_at,w.activated_at,w.created_at),w.scheduled_end_at,'[)') && tstzrange($3::timestamptz,$4::timestamptz,'[)'))`,scopes,except,start,end).Scan(&conflict)
 return conflict,err
}
func (h maintenanceOperation) Prepare(ctx context.Context,tx pgx.Tx,principal OpsPrincipal,request OpsPrepareRequest,lock bool)(OpsPreparedMaterial,error){
 var material OpsPreparedMaterial
 if !ValidOperationKey(request.Target.ID){return material,ErrOpsInvalid}
 input,err:=decodeMaintenanceInput(request.Input);if err!=nil{return material,err}
 current:=MaintenanceWindow{ID:request.Target.ID,State:"NOT_CREATED",Scopes:input.Scopes}
 // Activation and completion take guards before the window row, matching the
 // scheduled worker. Existing-window scopes are immutable after creation.
 if h.action=="END"||h.action=="CANCEL"{
  current,err=scanMaintenance(tx.QueryRow(ctx,`SELECT `+maintenanceColumns+` FROM ops.maintenance_windows w WHERE maintenance_id=$1`,request.Target.ID))
  if errors.Is(err,pgx.ErrNoRows){return material,ErrOpsNotFound};if err!=nil{return material,err}
 }
 if lock {if err=lockMaintenanceGuards(ctx,tx,current.Scopes);err!=nil{return material,err}}
 if criticalMaintenance(current.Scopes)!=h.critical{return material,ErrOpsInvalid}
 if h.action=="START"||h.action=="SCHEDULE" {
  if input.AnnouncementID!=""{var public bool;err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM content.announcements a JOIN content.announcement_revisions r ON r.announcement_id=a.announcement_id AND r.content_version=a.current_content_version WHERE a.announcement_id=$1 AND r.visibility='PUBLIC' AND a.state IN('PUBLISHED','SCHEDULED'))`,input.AnnouncementID).Scan(&public);if err!=nil{return material,err};if !public{return material,ErrOpsInvalid}}
  if input.CriticalNoticeID!=""{var exists bool;err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM ops.critical_notice_state WHERE critical_notice_id=$1)`,input.CriticalNoticeID).Scan(&exists);if err!=nil{return material,err};if exists{return material,ErrOpsConflict}}
  var exists bool;if err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM ops.maintenance_windows WHERE maintenance_id=$1)`,request.Target.ID).Scan(&exists);err!=nil{return material,err}
  if exists||request.Target.ExpectedVersion!="0"{return material,ErrOpsPreviewStale}
  var dbNow time.Time;if err=tx.QueryRow(ctx,`SELECT clock_timestamp()`).Scan(&dbNow);err!=nil{return material,err}
  if input.Start!=nil&&!input.Start.After(dbNow)||input.End!=nil&&!input.End.After(dbNow)||input.EstimatedEnd!=nil&&!input.EstimatedEnd.After(dbNow){return material,ErrOpsInvalid}
  lower:=dbNow;if input.Start!=nil{lower=*input.Start}
  overlap,err:=maintenanceOverlap(ctx,tx,current.Scopes,request.Target.ID,lower,input.End);if err!=nil{return material,err};if overlap{return material,ErrMaintenanceOverlap}
 }else{
  suffix:="";if lock{suffix=" FOR UPDATE"}
  current,err=scanMaintenance(tx.QueryRow(ctx,`SELECT `+maintenanceColumns+` FROM ops.maintenance_windows w WHERE maintenance_id=$1`+suffix,request.Target.ID));if err!=nil{return material,err}
  if strconv.FormatInt(current.Version,10)!=request.Target.ExpectedVersion{return material,ErrOpsPreviewStale}
  if h.action=="END"&&current.State!="ACTIVE"||h.action=="CANCEL"&&current.State!="SCHEDULED"{return material,ErrOpsConflict}
 }
 active:=[]MaintenanceWindow{}
 rows,err:=tx.Query(ctx,`SELECT `+maintenanceColumns+` FROM ops.maintenance_windows w WHERE w.state='ACTIVE' ORDER BY w.maintenance_id LIMIT 101`)
 if err!=nil{return material,err}
 for rows.Next(){item,e:=scanMaintenance(rows);if e!=nil{rows.Close();return material,e};active=append(active,item)};err=rows.Err();rows.Close();if err!=nil{return material,err};if len(active)>100{return material,ErrOpsUnavailable}
 before,_:=json.Marshal(current);currentState,_:=json.Marshal(map[string]any{"target":current,"active_maintenance":active});proposed,_:=json.Marshal(struct{Action string `json:"action"`;Input maintenanceInput `json:"input"`}{h.action,input})
 material=OpsPreparedMaterial{TargetVersion:strconv.FormatInt(current.Version,10),TargetLocator:request.Target.ID,
  Impact:OpsImpact{CurrentState:currentState,ProposedChange:proposed,Before:before,BlockingFacts:[]string{},ContinuingAcceptedWork:[]string{"已受理游戏的操作与结算","安全离桌与资金恢复","已有转账的查询与对账","登录安全和退出"},RelatedIDs:[]string{request.Target.ID},UnavailableMeasurements:[]string{"affected_users","pending_transfers","active_direct_play_rounds","active_poker_tables","active_poker_hands","pending_reward_claims","scheduled_ranking_publishes","scheduled_announcements"}}}
 return material,nil
}
func (h maintenanceOperation) Execute(ctx context.Context,tx pgx.Tx,principal OpsPrincipal,operation OpsOperation,material OpsPreparedMaterial)(OpsExecutionResult,error){
 input,err:=decodeMaintenanceInput(operation.inputPayload);if err!=nil{return OpsExecutionResult{},err}
 state:=map[string]string{"START":"ACTIVE","SCHEDULE":"SCHEDULED","END":"COMPLETED","CANCEL":"CANCELLED"}[h.action]
 if h.action=="START"||h.action=="SCHEDULE"{
  impact,err:=json.Marshal(material.Impact);if err!=nil{return OpsExecutionResult{},err};digest,err:=hex.DecodeString(operation.ImpactHash);if err!=nil{return OpsExecutionResult{},err}
  _,err=tx.Exec(ctx,`INSERT INTO ops.maintenance_windows(maintenance_id,state,reason,impact_snapshot,impact_hash,environment,scheduled_start_at,scheduled_end_at,estimated_end_at,activated_at,created_by,activated_by,operation_id,announcement_id)
   VALUES($1,$2,$3,$4,$5,$6,coalesce($7::timestamptz,clock_timestamp()),$8,$11,CASE WHEN $2='ACTIVE' THEN clock_timestamp() END,$9,CASE WHEN $2='ACTIVE' THEN $9::bigint END,$10,NULLIF($12,'')::uuid)`,operation.Target.ID,state,operation.Reason,impact,digest,operation.Environment,input.Start,input.End,principal.UserID,operation.OperationID,input.EstimatedEnd,input.AnnouncementID)
  if err!=nil{return OpsExecutionResult{},err}
  for _,scope:=range input.Scopes{if _,err=tx.Exec(ctx,`INSERT INTO ops.maintenance_window_scopes(maintenance_id,scope) VALUES($1,$2)`,operation.Target.ID,scope);err!=nil{return OpsExecutionResult{},err}}
  if input.CriticalNoticeID!=""{if _,err=tx.Exec(ctx,`INSERT INTO ops.critical_notice_state(critical_notice_id,maintenance_id) VALUES($1,$2)`,input.CriticalNoticeID,operation.Target.ID);err!=nil{return OpsExecutionResult{},err}}
  _,err=tx.Exec(ctx,`INSERT INTO ops.maintenance_jobs(job_key,maintenance_id,kind,due_at,status)
   SELECT 'maintenance:activate:'||maintenance_id::text,maintenance_id,'MAINTENANCE_ACTIVATE',scheduled_start_at,
    CASE WHEN state='ACTIVE' THEN 'COMPLETED' ELSE 'PENDING' END FROM ops.maintenance_windows WHERE maintenance_id=$1
   UNION ALL SELECT 'maintenance:end:'||maintenance_id::text,maintenance_id,'MAINTENANCE_END',scheduled_end_at,'PENDING'
    FROM ops.maintenance_windows WHERE maintenance_id=$1 AND scheduled_end_at IS NOT NULL`,operation.Target.ID)
  if err!=nil{return OpsExecutionResult{},err}
 }else{
  _,err=tx.Exec(ctx,`UPDATE ops.maintenance_windows SET state=$2,state_version=state_version+1,ended_at=clock_timestamp(),ended_by=$3 WHERE maintenance_id=$1`,operation.Target.ID,state,principal.UserID);if err!=nil{return OpsExecutionResult{},err}
  _,err=tx.Exec(ctx,`UPDATE ops.maintenance_jobs SET status='CANCELLED',updated_at=clock_timestamp() WHERE maintenance_id=$1 AND status='PENDING'`,operation.Target.ID);if err!=nil{return OpsExecutionResult{},err}
 }
 after,err:=scanMaintenance(tx.QueryRow(ctx,`SELECT `+maintenanceColumns+` FROM ops.maintenance_windows w WHERE maintenance_id=$1`,operation.Target.ID));if err!=nil{return OpsExecutionResult{},err}
 raw,_:=json.Marshal(after)
 if err=appendMaintenanceEvent(ctx,tx,after.ID,operation.OperationID,principal.UserID,state,"OPERATOR");err!=nil{return OpsExecutionResult{},err}
 return OpsExecutionResult{BeforeSnapshot:material.Impact.Before,AfterSnapshot:raw,Result:raw,RelatedBusinessID:after.ID},nil
}
func appendMaintenanceEvent(ctx context.Context,tx pgx.Tx,id,operationID string,actor int64,state,source string)error{
 _,err:=tx.Exec(ctx,`INSERT INTO ops.maintenance_events(maintenance_id,operation_id,actor_user_id,state,source) VALUES($1,$2,$3,$4,$5)`,id,operationID,actor,state,source);return err
}
func(s *Store) ReadOpsMaintenance(ctx context.Context,user int64)([]MaintenanceWindow,error){
 if _,err:=s.RequireOpsPermission(ctx,user,0,"maintenance.read");err!=nil{return nil,err}
 rows,err:=s.pool.Query(ctx,`SELECT `+maintenanceColumns+` FROM ops.maintenance_windows w ORDER BY created_at DESC,maintenance_id DESC LIMIT 100`);if err!=nil{return nil,err};defer rows.Close()
 result:=[]MaintenanceWindow{};for rows.Next(){item,err:=scanMaintenance(rows);if err!=nil{return nil,err};result=append(result,item)};return result,rows.Err()
}

// RunMaintenanceStep advances one authorized window through its atomically
// created schedule. Jobs cannot independently change the window's semantics.
func(s *Store) RunMaintenanceStep(ctx context.Context,environment string)(bool,error){
 if !slices.Contains([]string{"DEVELOPMENT","STAGING","PRODUCTION"},environment){return false,ErrOpsEnvironment}
 tx,err:=s.pool.Begin(ctx);if err!=nil{return false,err};defer rollback(tx)
 var id,initialState,operationID string;var actor,epoch int64
 err=tx.QueryRow(ctx,`SELECT w.maintenance_id::text,w.state,w.operation_id::text,w.created_by,o.actor_authz_epoch_snapshot
 FROM ops.maintenance_windows w JOIN ops.admin_operations o USING(operation_id)
 JOIN ops.maintenance_jobs j ON j.maintenance_id=w.maintenance_id AND j.status='PENDING'
 WHERE w.environment=$1 AND j.due_at<=clock_timestamp() AND ((w.state='SCHEDULED' AND j.kind='MAINTENANCE_ACTIVATE' AND w.scheduled_start_at<=clock_timestamp()) OR (w.state='ACTIVE' AND j.kind='MAINTENANCE_END' AND w.scheduled_end_at<=clock_timestamp()))
 ORDER BY CASE WHEN w.state='ACTIVE' THEN w.scheduled_end_at ELSE w.scheduled_start_at END,w.maintenance_id LIMIT 1`,environment).Scan(&id,&initialState,&operationID,&actor,&epoch)
 if errors.Is(err,pgx.ErrNoRows){return false,nil};if err!=nil{return false,err}
 allowed:=true
 if initialState=="SCHEDULED"{
  // Same actor -> scope -> target lock order as Ops Execute.
  _,err=requireOpsPermission(ctx,tx,actor,epoch,"maintenance.write",true)
  if errors.Is(err,ErrOpsForbidden)||errors.Is(err,ErrOpsAuthorizationStale){allowed=false}else if err!=nil{return false,err}
 }
 current,err:=scanMaintenance(tx.QueryRow(ctx,`SELECT `+maintenanceColumns+` FROM ops.maintenance_windows w WHERE maintenance_id=$1`,id));if err!=nil{return false,err}
 if err=lockMaintenanceGuards(ctx,tx,current.Scopes);err!=nil{return false,err}
 current,err=scanMaintenance(tx.QueryRow(ctx,`SELECT `+maintenanceColumns+` FROM ops.maintenance_windows w WHERE maintenance_id=$1 FOR UPDATE SKIP LOCKED`,id))
 if errors.Is(err,pgx.ErrNoRows){return false,nil};if err!=nil{return false,err};if current.State!=initialState{return false,nil}
 if current.Environment!=environment{return false,ErrOpsEnvironment}
 var dbNow time.Time;if err=tx.QueryRow(ctx,`SELECT clock_timestamp()`).Scan(&dbNow);err!=nil{return false,err}
 state:="COMPLETED"
 if initialState=="SCHEDULED"{
  state="ACTIVE"
  if !allowed||current.ScheduledStart==nil||current.ScheduledStart.After(dbNow)||current.ScheduledEnd!=nil&&!current.ScheduledEnd.After(dbNow){state="ACTIVATION_FAILED"}else{
   overlap,err:=maintenanceOverlap(ctx,tx,current.Scopes,id,dbNow,current.ScheduledEnd);if err!=nil{return false,err};if overlap{state="ACTIVATION_FAILED"}
  }
 }
 _,err=tx.Exec(ctx,`UPDATE ops.maintenance_windows SET state=$2,state_version=state_version+1,
 activated_at=CASE WHEN $2='ACTIVE' THEN clock_timestamp() ELSE activated_at END,
 activated_by=CASE WHEN $2='ACTIVE' THEN $3::bigint ELSE activated_by END,
 ended_at=CASE WHEN $2 IN('COMPLETED','ACTIVATION_FAILED') THEN clock_timestamp() ELSE ended_at END,
 ended_by=CASE WHEN $2='COMPLETED' THEN $3::bigint ELSE ended_by END WHERE maintenance_id=$1`,id,state,actor)
 if err!=nil{return false,err}
 jobKind:="MAINTENANCE_END";jobState:="COMPLETED";if initialState=="SCHEDULED"{jobKind="MAINTENANCE_ACTIVATE"};if state=="ACTIVATION_FAILED"{jobState="FAILED"}
 tag,err:=tx.Exec(ctx,`UPDATE ops.maintenance_jobs SET status=$3,updated_at=clock_timestamp() WHERE maintenance_id=$1 AND kind=$2 AND status='PENDING'`,id,jobKind,jobState)
 if err!=nil{return false,err};if tag.RowsAffected()!=1{return false,ErrOpsConflict}
 if state=="ACTIVATION_FAILED"{if _,err=tx.Exec(ctx,`UPDATE ops.maintenance_jobs SET status='CANCELLED',updated_at=clock_timestamp() WHERE maintenance_id=$1 AND status='PENDING'`,id);err!=nil{return false,err}}
 if err=appendMaintenanceEvent(ctx,tx,id,operationID,actor,state,"SCHEDULE");err!=nil{return false,err}
 if err=tx.Commit(ctx);err!=nil{return false,err};return true,nil
}
