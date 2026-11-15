#!/usr/bin/env python3
"""Pins argument validation in docker.sh and fleet-systemctl.sh: run
`python3 scripts/test_fleet_script_args.py` (no pytest).

Both scripts splice arguments into a remote shell string, and agents run them
without a permission prompt, so every case here is a command that would
otherwise execute on fleet hosts.
"""
import os
import stat
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))


def run(script, *args):
    with tempfile.TemporaryDirectory() as stubs:
        log = os.path.join(stubs, "ssh.log")
        ssh = os.path.join(stubs, "ssh")
        with open(ssh, "w") as f:
            f.write(f'#!/usr/bin/env bash\necho "$*" >>"{log}"\n')
        os.chmod(ssh, os.stat(ssh).st_mode | stat.S_IEXEC)
        env = dict(os.environ, PATH=f"{stubs}:{os.environ['PATH']}")
        rc = subprocess.run([os.path.join(HERE, script), *args], env=env,
                            capture_output=True, timeout=60).returncode
        return rc, os.path.exists(log)


INJECTIONS = [
    ("docker.sh", "logs", "x;id"),
    ("docker.sh", "status", "$(id)"),
    ("docker.sh", "where", "x y"),
    ("docker.sh", "logs", "nginx", "ocean", "5;id"),
    ("fleet-systemctl.sh", "x';id;'"),
    ("fleet-systemctl.sh", "nginx", "`id`"),
    ("fleet-systemctl.sh", "host", "a|b"),
]

ALLOWED = [
    ("docker.sh", "where", "nginx"),
    ("fleet-systemctl.sh", "agentbox-rc@repositories-root.service"),
]

failed = 0
for case in INJECTIONS:
    rc, reached = run(*case)
    if rc == 0 or reached:
        print(f"FAIL injection_reaches_ssh {case}: rc={rc} reached_ssh={reached}")
        failed = 1
for case in ALLOWED:
    _, reached = run(*case)
    if not reached:
        print(f"FAIL valid_name_rejected {case}: never reached ssh")
        failed = 1

print("all passed" if not failed else "")
sys.exit(failed)
