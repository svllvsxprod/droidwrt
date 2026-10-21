#!/usr/bin/env python3
from pathlib import Path
import subprocess,secrets,os,zipfile
root=Path(__file__).resolve().parent
out=root/'build';out.mkdir(exist_ok=True)
private=root.parent/'work/private';private.mkdir(exist_ok=True);private.chmod(0o700)
jdk=Path(os.environ['JAVA_HOME'])/'bin'
sdk=Path(os.environ['ANDROID_HOME']);bt=sdk/'build-tools/36.0.0';android=sdk/'platforms/android-36/android.jar'
def run(args,**kw):subprocess.run([str(a) for a in args],check=True,**kw)
for d in ['gen','classes','dex']:(out/d).mkdir(exist_ok=True)
run([bt/'aapt2','compile','--dir',root/'res','-o',out/'resources.zip'])
run([bt/'aapt2','link','-o',out/'resources.apk','--manifest',root/'AndroidManifest.xml','-I',android,'--java',out/'gen',out/'resources.zip'])
sources=list((root/'src').rglob('*.java'))+list((out/'gen').rglob('*.java'))
jar=root/'libs/zxing-core-3.5.4.jar'
run([jdk/'javac','-encoding','UTF-8','--release','8','-classpath',str(android)+os.pathsep+str(jar),'-d',out/'classes',*sources])
run([jdk/'java','-cp',bt/'lib/d8.jar','com.android.tools.r8.D8','--lib',android,'--min-api','28','--output',out/'dex',*list((out/'classes').rglob('*.class')),jar])
with zipfile.ZipFile(out/'resources.apk') as source,zipfile.ZipFile(out/'unsigned.apk','w') as target:
 for name in source.namelist():target.writestr(name,source.read(name))
 for dex in (out/'dex').glob('*.dex'):target.write(dex,dex.name)
run([bt/'zipalign','-f','4',out/'unsigned.apk',out/'aligned.apk'])
key=private/'panel-signing.p12';pwfile=private/'panel-signing-password'
if not key.exists():
 password=secrets.token_urlsafe(32);pwfile.write_text(password);pwfile.chmod(0o600)
 env=dict(os.environ,DROIDWRT_SIGNING_PASSWORD=password)
 run([jdk/'keytool','-genkeypair','-keystore',key,'-storetype','PKCS12','-storepass:env','DROIDWRT_SIGNING_PASSWORD','-keypass:env','DROIDWRT_SIGNING_PASSWORD','-alias','droidwrt','-keyalg','RSA','-keysize','3072','-validity','10000','-dname','CN=DroidWRT'],env=env)
 key.chmod(0o600)
run([jdk/'java','-jar',bt/'lib/apksigner.jar','sign','--ks',key,'--ks-pass','file:'+str(pwfile),'--out',out/'DroidWRT.apk',out/'aligned.apk'])
run([jdk/'java','-jar',bt/'lib/apksigner.jar','verify',out/'DroidWRT.apk'])
print('Built and verified DroidWRT.apk')
