#!/usr/bin/env python3
"""Isolated actual SQL-server image preflight; creates only owned local Docker fixtures.

Requires the locally built aurora-go-execution:sop-preflight image. Never publishes
ports or contacts dev/shared PostgreSQL. Refuses a pre-existing fixture network.
"""
import json,os,secrets,subprocess,time
from pathlib import Path
os.umask(0o077)
ROOT=Path('/tmp/aurora-merchant-sop-completion-20261021/go-runtime')
OUT=ROOT/'sql-image-check';OUT.mkdir(mode=0o700,exist_ok=True)
NET='aurora-sop-sql-check-20261021'
PG='aurora-sop-sql-check-pg-20261021'
SERVER='aurora-sop-sql-check-server-20261021'
OWNER='aurora.sop.fixture=sql-image-check-20261021'
ADMIN='temporalio/admin-tools:1.32.0@sha256:a9f84fb9a374b2374fe2e67c8efc0468ff3f1c66c8a0b14597ec86e349e62bca'
IMAGE='temporalio/server:1.32.0@sha256:c3e752127759616bb1615e0f9ba0e21635aeb5fdeb922de4f371c350955f46ae'
token=secrets.token_urlsafe(32)
dynamic=OUT/'dynamic.yaml';dynamic.write_text('{}\n');dynamic.chmod(0o644)
def envfile(name,values):
    path=OUT/name;path.write_text(''.join(k+'='+v+'\n' for k,v in values.items()));path.chmod(0o600);return str(path)
def docker(*args,check=True):
    result=subprocess.run(['docker',*args],capture_output=True,text=True,timeout=120)
    if check and result.returncode:
        (OUT/'failure.log').write_text((result.stdout+result.stderr).replace(token,'[redacted]'))
        raise RuntimeError('local SQL image check failed; private failure.log records sanitized diagnostics')
    return result
pgfile=envfile('pg.env',{'POSTGRES_USER':'probe_owner','POSTGRES_PASSWORD':token})
adminfile=envfile('admin.env',{'SQL_USER':'probe_owner','SQL_PASSWORD':token,'POSTGRES_SEEDS':'postgres','TEMPORAL_ADDRESS':'temporal:7233'})
serverfile=envfile('server.env',{'DB':'postgres12','DBNAME':'aurora_temporal_dev','VISIBILITY_DBNAME':'aurora_temporal_visibility_dev','POSTGRES_SEEDS':'postgres','POSTGRES_USER':'probe_owner','POSTGRES_PWD':token,'DB_PORT':'5432','BIND_ON_IP':'0.0.0.0','TEMPORAL_ALLOW_NO_AUTH':'true','SQL_MAX_CONNS':'1','SQL_MAX_IDLE_CONNS':'1','SQL_VIS_MAX_CONNS':'1','SQL_VIS_MAX_IDLE_CONNS':'1','LOG_LEVEL':'warn','NUM_HISTORY_SHARDS':'4','DYNAMIC_CONFIG_FILE_PATH':'/etc/temporal/dynamic/config.yaml'})
created=[];netcreated=False
try:
    if docker('network','inspect',NET,check=False).returncode==0:raise RuntimeError('refusing existing SQL check network')
    docker('network','create','--internal','--label',OWNER,NET);netcreated=True
    docker('run','-d','--name',PG,'--label',OWNER,'--network',NET,'--network-alias','postgres','--cpus','1','--memory','512m','--env-file',pgfile,'postgres@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15');created.append(PG)
    for _ in range(60):
        if docker('exec',PG,'pg_isready','-h','127.0.0.1','-U','probe_owner',check=False).returncode==0:break
        time.sleep(.2)
    else:raise RuntimeError('private PostgreSQL did not become ready')
    for name in ('aurora_temporal_dev','aurora_temporal_visibility_dev'):docker('exec',PG,'createdb','-U','probe_owner',name)
    draft=Path(__file__).resolve().parents[2]/'deploy/dev/11-schema-job.yml'
    parsed=subprocess.run(['ruby','-ryaml','-rjson','-e','puts JSON.dump(YAML.load_stream(File.read(ARGV[0])))',str(draft)],capture_output=True,text=True,check=True)
    script=json.loads(parsed.stdout)[0]['spec']['template']['spec']['containers'][0]['command'][2]
    docker('run','--rm','--platform','linux/amd64','--label',OWNER,'--network',NET,'--env-file',adminfile,'--cap-drop','ALL','--security-opt','no-new-privileges','--entrypoint','/bin/sh',ADMIN,'-ec',script)
    print('Actual pinned admin-tools initialized both isolated PostgreSQL18.4 schemas.',flush=True)
    docker('run','-d','--platform','linux/amd64','--name',SERVER,'--label',OWNER,'--network',NET,'--network-alias','temporal','-v',str(dynamic)+':/etc/temporal/dynamic/config.yaml:ro','--read-only','--tmpfs','/tmp:rw,noexec,nosuid,uid=1000,gid=1000','--cap-drop','ALL','--security-opt','no-new-privileges','--cpus','2','--memory','2g','--env-file',serverfile,IMAGE);created.append(SERVER)
    for _ in range(60):
        check=docker('run','--rm','--platform','linux/amd64','--label',OWNER,'--network',NET,'-e','TEMPORAL_HOST_PORT=temporal:7233','--entrypoint','/app/health-probe','aurora-go-execution:sop-preflight','temporal',check=False)
        if check.returncode==0:break
        if docker('inspect','-f','{{.State.Running}}',SERVER).stdout.strip()!='true':raise RuntimeError('pinned server exited before readiness')
        time.sleep(.2)
    else:raise RuntimeError('pinned SQL-backed server health timeout')
    print('Actual pinned non-root, read-only SQL-backed server passed Go gRPC health probe.',flush=True)
    docker('run','--rm','--platform','linux/amd64','--label',OWNER,'--network',NET,'--env-file',adminfile,'--entrypoint','temporal',ADMIN,'operator','namespace','create','--address','temporal:7233','--namespace','aurora-merchant-sop-dev','--retention','7d')
    # Run the retained legacy workflow branch to a real Temporal completion. No callback stub is used.
    worker='aurora-sop-sql-check-workflow-20261021'
    agent='aurora-sop-sql-check-agent-20261021'
    for name,role in ((worker,'service-worker'),(agent,'agent-executor')):
        docker('run','-d','--platform','linux/amd64','--name',name,'--label',OWNER,'--network',NET,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','-e','TEMPORAL_HOST_PORT=temporal:7233','-e','TEMPORAL_NAMESPACE=aurora-merchant-sop-dev','-e','SOP_BASE_URL=http://127.0.0.1:1/sop','--entrypoint','/app/'+role,'aurora-go-execution:sop-preflight');created.append(name)
    payload=json.dumps({'dispatchId':'sql-pool-probe-20261021','processInstanceId':'sql-pool-probe','nodeInstanceId':'echo','nodeKey':'echo','taskType':'system.echo','input':{'message':'isolated low-pool probe'},'timeoutSeconds':30,'retry':{'maxAttempts':1}})
    execution=docker('run','--rm','--platform','linux/amd64','--label',OWNER,'--network',NET,'--entrypoint','temporal',ADMIN,'workflow','execute','--address','temporal:7233','--namespace','aurora-merchant-sop-dev','--workflow-id','sql-pool-probe-20261021','--type','ServiceDispatchWorkflow','--task-queue','service-worker','--input',payload)
    (OUT/'workflow.log').write_text(execution.stdout+execution.stderr)
    observations=[]
    for _ in range(12):
        q=docker('exec',PG,'psql','-U','probe_owner','-d','postgres','-tA','-c',"SELECT json_build_object('total',count(*),'active',count(*) FILTER (WHERE state='active'),'idle',count(*) FILTER (WHERE state='idle'),'byDatabase', (SELECT json_object_agg(datname,n) FROM (SELECT datname,count(*) n FROM pg_stat_activity WHERE datname IN ('aurora_temporal_dev','aurora_temporal_visibility_dev') GROUP BY datname) x)) FROM pg_stat_activity WHERE datname IN ('aurora_temporal_dev','aurora_temporal_visibility_dev');")
        observations.append(json.loads(q.stdout));time.sleep(1)
    (OUT/'connections.json').write_text(json.dumps(observations,indent=2)+'\n')
    print('Low-pool Temporal actual workflow completed; observed SQL connections '+str([x['total'] for x in observations]),flush=True)
    summary={'postgres' :'18.4-alpine','temporal':'1.32.0','isolatedDatabases':2,'schemaJobScript':'passed with actual admin-tools','readOnlyNonRootServer':True,'grpcHealth':'SERVING','namespaceRetention':'7d','sqlMaxConnsPerStore':1,'sqlMaxIdleConnsPerStore':1,'observedConnectionSamples':observations,'workflow':'legacy ServiceDispatchWorkflow completed against real SQL Temporal; no callback stub','scope':'private local Docker network, no published ports; not a dev deployment or NetworkPolicy test'}
    (OUT/'evidence.json').write_text(json.dumps(summary,indent=2)+'\n')
    print(json.dumps(summary),flush=True)
finally:
    if SERVER in created:
        logs=docker('logs',SERVER,check=False)
        (OUT/'server.log').write_text((logs.stdout+logs.stderr).replace(token,'[redacted]'))
    for name in reversed(created):docker('rm','-f',name,check=False)
    if netcreated:docker('network','rm',NET,check=False)
    for name in ('pg.env','admin.env','server.env'):(OUT/name).unlink(missing_ok=True)
    print('Owned SQL-check containers/network and private temporary credentials cleaned.',flush=True)
