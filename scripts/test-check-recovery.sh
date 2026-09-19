#!/usr/bin/env python3
import json, os
from pathlib import Path
import subprocess, tempfile
CHECK=Path(__file__).resolve().with_name("check-recovery.sh")
FIX='''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
s=json.loads(Path(os.environ["F"]).read_text());a=sys.argv[1:];assert a.pop(0)=="--json";n=" ".join(a)
if n in s.get("fail",[]):sys.exit(8)
if n.endswith("restart"):s["r"].append(n);s["x"]=1;Path(os.environ["F"]).write_text(json.dumps(s));print("{}")
elif n=="config show":print('{"executor":{"model":"fixture"}}')
elif n=="telegram status":print(json.dumps(s["t"]))
elif n=="controller status":
 c=s["c"]
 if s.get("delay") and s.get("x"):c={"status":dict(c["status"],controller_ready=True,worker_running=True)}
 print(json.dumps(c))
elif n=="monitor inspect":print(json.dumps(s["m"]))
elif n=="monitor status":print(json.dumps(s["ms"]))
else:sys.exit(2)
'''
def ok():return {"r":[],"t":{"configured":True,"paired":True,"enabled":True,"receiver_alive":True,"supervisor_alive":True,"state_readable":True,"receiver":{"poll":"connected"}},"c":{"status":{"controller_running":True,"controller_ready":True,"controller_managed":True,"worker_requested":True,"worker_running":True}},"m":{"controller":{"intent_readable":True,"enabled":True},"monitor_service":{"intent_readable":True,"enabled":True,"loaded":True}},"ms":{"process_alive":True,"fresh":True}}
def run(s,args,code,restarts=()):
 with tempfile.TemporaryDirectory() as d:
  d=Path(d);f=d/"f";b=d/"chunsu";f.write_text(json.dumps(s));b.write_text(FIX);b.chmod(0o755);q=subprocess.run([str(CHECK),*args,"--bin",str(b),"--report-dir",str(d/"rpt")],env={**os.environ,"F":str(f)},capture_output=True)
  assert q.returncode==code;(p,)=list((d/"rpt").glob("*.json"));assert json.loads(p.read_text())["success"]==(code==0);assert p.stat().st_mode&0o777==0o600;assert json.loads(f.read_text())["r"]==list(restarts)
s=ok();s["delay"]=1;run(s,["--exercise"],0,("controller restart","telegram restart","monitor restart"))
s=ok();s["t"].update(enabled=False,receiver_alive=False,supervisor_alive=False);s["c"]["status"].update(controller_running=False,controller_ready=False);s["m"]["controller"]["enabled"]=False;s["m"]["monitor_service"]["enabled"]=False;s["ms"].update(process_alive=False,fresh=False);run(s,["--exercise"],0)
s=ok();s["t"]["enabled"]=False;run(s,[],1)
s=ok();s["t"]={"configured":False};run(s,[],0)
s=ok();s["c"]["status"]["active_job"]="job";run(s,["--exercise"],1)
s=ok();s["t"]["receiver"]["poll"]="waiting";run(s,[],1)
s=ok();s["fail"]=["controller status"];run(s,[],1)
print("check-recovery fixtures passed")
