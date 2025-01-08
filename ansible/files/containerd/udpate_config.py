import re
import sys

def update_config_file(filepath, set_nvidia_runtime=False):
    with open(filepath, 'r') as file:
        lines = file.readlines()

    in_section = False
    systemd_cgroup_exists = False

    for i, line in enumerate(lines):
        if re.match(r'\s*\[plugins\."io\.containerd\.runtime\.v1\.linux"\]', line):
            print('matched [plugins."io.containerd.runtime.v1.linux"]')
            in_section = True
        elif re.match(r'^\[plugins\."io\.containerd\..*"\]', line):  # Detect the start of the next section
            if in_section and not systemd_cgroup_exists:
                # Insert systemd_cgroup = true before the next section starts
                lines.insert(i, '    systemd_cgroup = true\n')
                print("Inserted systemd_cgroup = true before the next section")
            in_section = False

        if in_section:
            # Ensure systemd_cgroup = true
            if re.match(r'^\s*systemd_cgroup\s*=', line):
                print("matched systemd_cgroup")
                lines[i] = '    systemd_cgroup = true\n'
                systemd_cgroup_exists = True
            elif set_nvidia_runtime and re.match(r'^\s*runtime\s*=\s*', line):
                lines[i] = '    runtime = "nvidia-container-runtime"\n'

    # If systemd_cgroup was not found in the section, add it at the end
    if in_section and not systemd_cgroup_exists:
        for i in reversed(range(len(lines))):
            if re.match(r'\s*\[plugins\."io\.containerd\.runtime\.v1\.linux"\]', lines[i]):
                # Add the systemd_cgroup line after the last line of the section
                lines.insert(i + 1, '    systemd_cgroup = true\n')
                print("Added systemd_cgroup at the end of the section")
                break

    with open(filepath, 'w') as file:
        print("writing file")
        file.writelines(lines)

if __name__ == "__main__":
    # Expecting filepath as the first argument and nvidia_gpu as an optional second argument
    filepath = sys.argv[1]
    set_nvidia_runtime = bool(int(sys.argv[2])) if len(sys.argv) > 2 else False
    update_config_file(filepath, set_nvidia_runtime)
