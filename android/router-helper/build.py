from pathlib import Path
import subprocess, os
root=Path(__file__).resolve().parent
out=root.parents[1]/'work/network/android-helper'
out.mkdir(parents=True, exist_ok=True)
jdk=Path(os.environ['JAVA_HOME'])/'bin'
sdk=Path(os.environ['ANDROID_HOME'])
subprocess.run([str(jdk/'javac'),'--release','8','-cp',str(sdk/'platforms/android-36/android.jar'),'-d',str(out),str(root/'RouterWifi.java')],check=True)
classes=list(out.glob('RouterWifi*.class'))
subprocess.run([str(jdk/'java'),'-cp',str(sdk/'build-tools/36.0.0/lib/d8.jar'),'com.android.tools.r8.D8','--lib',str(sdk/'platforms/android-36/android.jar'),'--min-api','28','--output',str(out),*[str(p) for p in classes]],check=True)
(root.parents[1]/'openwrt/router-module/router-wifi.dex').write_bytes((out/'classes.dex').read_bytes())
