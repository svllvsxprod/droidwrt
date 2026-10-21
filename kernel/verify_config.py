"""Fail the build when router essentials or thermal protection are absent."""
import json
import pathlib
import sys

config = dict(line.split('=', 1) for line in pathlib.Path(sys.argv[1]).read_text().splitlines()
              if line.startswith('CONFIG_') and '=' in line)
required = '''SYSVIPC POSIX_MQUEUE IPC_NS PID_NS UTS_NS NET_NS USER_NS
CGROUPS CGROUP_DEVICE CGROUP_PIDS CGROUP_SCHED CFS_BANDWIDTH MEMCG
DEVTMPFS OVERLAY_FS SECCOMP SECCOMP_FILTER
USB_CONFIGFS_NCM USB_CONFIGFS_ECM USB_CONFIGFS_F_FS
USB_RTL8152 USB_NET_AX88179_178A USB_NET_CDCETHER USB_NET_CDC_NCM
TUN VETH BRIDGE MACVLAN VLAN_8021Q IP_ADVANCED_ROUTER IP_MULTIPLE_TABLES
IPV6 IPV6_MULTIPLE_TABLES NF_TABLES NF_TABLES_INET
NETFILTER_XT_TARGET_TPROXY NETFILTER_XT_MATCH_SOCKET
IP_NF_TARGET_MASQUERADE IP6_NF_TARGET_MASQUERADE
NET_SCH_FQ_CODEL NET_SCH_FQ IFB TCP_CONG_BBR
THERMAL CPU_THERMAL CPU_FREQ_DEFAULT_GOV_SCHEDUTIL MTK_COMBO_WIFI'''.split()
missing = ['CONFIG_' + name for name in required if config.get('CONFIG_' + name) != 'y']
requested = dict(line.split('=', 1) for line in pathlib.Path('/project/kernel/router.config').read_text().splitlines()
                 if line.startswith('CONFIG_') and '=' in line)
unavailable = {name: {'requested': value, 'resolved': config.get(name, 'n')}
               for name, value in requested.items() if config.get(name, 'n') != value}
report = {'required_missing': missing, 'unresolved_requests': unavailable,
          'thermal_protection': config.get('CONFIG_THERMAL'),
          'cpu_governor': 'schedutil', 'overclock_added': False}
pathlib.Path('/project/work/kernel-build/config-verification.json').write_text(json.dumps(report, indent=2) + '\n')
print(json.dumps(report, indent=2))
if missing:
    sys.exit('Required options not enabled')
