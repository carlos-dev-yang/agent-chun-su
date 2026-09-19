#!/usr/bin/env python3
"""Check Chun-su recovery readiness without exposing configuration contents."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import uuid

POLL_ATTEMPTS = 30
POLL_INTERVAL_SECONDS = 0.2
COMMAND_TIMEOUT_SECONDS = 60

class RecoveryError(Exception):
    pass

def default_bin():
    return os.environ.get("CHUNSU_BIN", str(Path.home() / ".local/bin/chunsu"))

def default_report_dir():
    state_home = os.environ.get("XDG_STATE_HOME")
    base = Path(state_home) if state_home else Path.home() / ".local" / "state"
    return base / "chunsu" / "recovery"

def command(binary, *args):
    try:
        result = subprocess.run([binary, "--json", *args], text=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=False, timeout=COMMAND_TIMEOUT_SECONDS)
    except OSError as exc:
        raise RecoveryError("command could not be started: " + " ".join(args)) from exc
    except subprocess.TimeoutExpired as exc:
        raise RecoveryError("command timed out: " + " ".join(args)) from exc
    if result.returncode:
        raise RecoveryError("command failed: " + " ".join(args))
    try:
        value = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise RecoveryError("command returned invalid JSON: " + " ".join(args)) from exc
    if not isinstance(value, dict):
        raise RecoveryError("command returned invalid JSON object: " + " ".join(args))
    return value

def boolean(value, path):
    if not isinstance(value, bool):
        raise RecoveryError("status is missing boolean " + path)
    return value

def string(value, path):
    if not isinstance(value, str):
        raise RecoveryError("status is missing string " + path)
    return value

def object_at(value, key, path):
    child = value.get(key)
    if not isinstance(child, dict):
        raise RecoveryError("status is missing object " + path + "." + key)
    return child

def telegram_state(status):
    configured = boolean(status.get("configured"), "telegram.configured")
    if not configured:
        return {"intent": "unconfigured", "ready": False, "restartable": False}
    enabled = boolean(status.get("enabled"), "telegram.enabled")
    paired = boolean(status.get("paired"), "telegram.paired")
    supervisor = boolean(status.get("supervisor_alive"), "telegram.supervisor_alive")
    receiver = boolean(status.get("receiver_alive"), "telegram.receiver_alive")
    readable = boolean(status.get("state_readable"), "telegram.state_readable")
    health = object_at(status, "receiver", "telegram")
    poll = string(health.get("poll"), "telegram.receiver.poll")
    if not enabled:
        return {"intent": "stopped", "ready": not (supervisor or receiver), "restartable": False}
    ready = paired and supervisor and receiver and readable and poll == "connected"
    return {"intent": "running", "ready": ready, "restartable": ready}

def controller_state(status, monitor):
    result = object_at(status, "status", "controller")
    running = boolean(result.get("controller_running"), "controller.status.controller_running")
    ready = boolean(result.get("controller_ready"), "controller.status.controller_ready")
    managed = boolean(result.get("controller_managed"), "controller.status.controller_managed")
    requested = boolean(result.get("worker_requested"), "controller.status.worker_requested")
    worker_running = boolean(result.get("worker_running"), "controller.status.worker_running")
    active_job = result.get("active_job", "")
    string(active_job, "controller.status.active_job")
    controller = object_at(monitor, "controller", "monitor")
    readable = boolean(controller.get("intent_readable"), "monitor.controller.intent_readable")
    enabled = boolean(controller.get("enabled"), "monitor.controller.enabled")
    if not readable:
        raise RecoveryError("controller enabled intent is unreadable")
    if not enabled:
        return {"intent": "stopped", "ready": not running, "restartable": False, "worker_requested": requested, "worker_running": worker_running}
    ready_all = running and ready and managed and (not requested or worker_running)
    return {"intent": "running", "ready": ready_all, "restartable": running and ready and managed, "worker_requested": requested, "worker_running": worker_running}

def monitor_state(inspect, status):
    service = object_at(inspect, "monitor_service", "monitor")
    readable = boolean(service.get("intent_readable"), "monitor.monitor_service.intent_readable")
    enabled = boolean(service.get("enabled"), "monitor.monitor_service.enabled")
    process_alive = boolean(status.get("process_alive"), "monitor.status.process_alive")
    fresh = boolean(status.get("fresh"), "monitor.status.fresh")
    if not readable:
        raise RecoveryError("monitor enabled intent is unreadable")
    if not enabled:
        return {"intent": "stopped", "ready": not process_alive, "restartable": False}
    return {"intent": "running", "ready": process_alive and fresh,
            "restartable": process_alive and fresh}

def snapshot(binary):
    config = command(binary, "config", "show")
    telegram = command(binary, "telegram", "status")
    controller = command(binary, "controller", "status")
    monitor = command(binary, "monitor", "inspect")
    monitor_status = command(binary, "monitor", "status")
    return {"configuration_sha256": hashlib.sha256(json.dumps(config, sort_keys=True, separators=(",", ":")).encode()).hexdigest(), "telegram": telegram, "controller": controller, "monitor": monitor, "monitor_status": monitor_status}

def assess(sample):
    return {"telegram": telegram_state(sample["telegram"]), "controller": controller_state(sample["controller"], sample["monitor"]), "monitor": monitor_state(sample["monitor"], sample["monitor_status"])}

def wait_for(binary, component):
    for _ in range(POLL_ATTEMPTS):
        sample = snapshot(binary)
        states = assess(sample)
        # Controller readiness includes requested worker state, before later actions.
        if states[component]["ready"]:
            return sample, states
        time.sleep(POLL_INTERVAL_SECONDS)
    raise RecoveryError(component + " did not converge before the recovery check deadline")

def private_report_path(directory):
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    try:
        directory.chmod(0o700)
    except OSError:
        pass
    return directory / ("recovery-" + time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()) + "-" + uuid.uuid4().hex + ".json")

def write_report(path, report):
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    with os.fdopen(os.open(path, flags, 0o600), "w", encoding="utf-8") as output:
        json.dump(report, output, indent=2, sort_keys=True)
        output.write("\n")

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exercise", action="store_true", help="restart only already-ready managed services")
    parser.add_argument("--bin", default=default_bin(), help="path to the installed chunsu binary")
    parser.add_argument("--report-dir", type=Path, default=default_report_dir(), help="private directory for a unique JSON report")
    args = parser.parse_args()
    report_path = private_report_path(args.report_dir.expanduser().resolve())
    report = {"version": 1, "exercise": args.exercise, "success": False}
    try:
        if not os.path.isfile(args.bin) or not os.access(args.bin, os.X_OK):
            raise RecoveryError("installed chunsu binary is not executable")
        before = snapshot(args.bin)
        states = assess(before)
        report["configuration_sha256_before"] = before["configuration_sha256"]
        original_states = states
        report["before"] = {name: before[name] for name in ("telegram", "controller", "monitor", "monitor_status")}
        report["readiness_before"] = states
        if args.exercise:
            if before["controller"]["status"].get("active_job", ""):
                raise RecoveryError("recovery exercise refused while controller has an active job")
            if any(state["intent"] != "unconfigured" and not state["ready"] for state in states.values()):
                raise RecoveryError("recovery exercise refused until intended services are ready")
            # A worker follows the controller; it is never restarted by this tool.
            for component, command_name in (("controller", "controller"), ("telegram", "telegram"), ("monitor", "monitor")):
                if states[component]["restartable"]:
                    command(args.bin, command_name, "restart")
                    before, states = wait_for(args.bin, component)
            after = snapshot(args.bin)
        else:
            after = snapshot(args.bin)
        after_states = assess(after)
        report["configuration_sha256_after"] = after["configuration_sha256"]
        report["after"] = {name: after[name] for name in ("telegram", "controller", "monitor", "monitor_status")}
        report["readiness_after"] = after_states
        if report["configuration_sha256_before"] != report["configuration_sha256_after"]:
            raise RecoveryError("configuration changed during recovery validation")
        if any(after_states[name]["intent"] != original_states[name]["intent"] for name in original_states):
            raise RecoveryError("service intent changed during recovery validation")
        if after_states["controller"]["worker_requested"] != original_states["controller"]["worker_requested"]:
            raise RecoveryError("worker intent changed during recovery validation")
        for name, state in after_states.items():
            if state["intent"] != "unconfigured" and not state["ready"]:
                raise RecoveryError(name + " is not in its intended state")
        report["success"] = True
    except RecoveryError as exc:
        report["error"] = str(exc)
    write_report(report_path, report)
    if report["success"]:
        print("Recovery report written to " + str(report_path))
        return 0
    print("Recovery check failed; report written to " + str(report_path), file=sys.stderr)
    return 1

if __name__ == "__main__":
    sys.exit(main())
