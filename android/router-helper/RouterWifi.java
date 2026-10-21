import android.os.*;
import org.json.JSONObject;
import java.lang.reflect.*;
import java.nio.file.*;
import java.util.concurrent.*;

public final class RouterWifi {
 static Object service(String name,String cls)throws Exception{
  IBinder binder=(IBinder)Class.forName("android.os.ServiceManager").getMethod("getService",String.class).invoke(null,name);
  return Class.forName(cls+"$Stub").getMethod("asInterface",IBinder.class).invoke(null,binder);
 }
 public static void main(String[] args)throws Exception {
  if(args.length==0)throw new IllegalArgumentException();
  Object wifi=service("wifi","android.net.wifi.IWifiManager");Class<?> manager=Class.forName("android.net.wifi.IWifiManager");
  if(args[0].equals("stop")){System.out.println(manager.getMethod("stopSoftAp").invoke(wifi));return;}
  if(args[0].equals("start")||args[0].equals("validate")){
   Class<?> builder=Class.forName("android.net.wifi.SoftApConfiguration$Builder"),config=Class.forName("android.net.wifi.SoftApConfiguration");Object b=builder.getConstructor().newInstance();
   JSONObject settings=new JSONObject(new String(Files.readAllBytes(Paths.get(args[0].equals("validate")&&args.length==2?args[1]:"/data/adb/modules/droidwrt_router/hotspot-config.json")),"UTF-8"));
   String security=settings.getString("security"),band=settings.getString("band");
   int type=security.equals("wpa2")?1:security.equals("wpa2-wpa3")?2:security.equals("wpa3")?3:security.equals("open")?0:-1;
   if(type<0||(!band.equals("2.4")&&!band.equals("5")))throw new IllegalArgumentException("Unsupported Wi-Fi configuration");
   builder.getMethod("setSsid",String.class).invoke(b,settings.getString("ssid"));
   builder.getMethod("setPassphrase",String.class,int.class).invoke(b,type==0?null:settings.getString("password"),type);
   builder.getMethod("setBand",int.class).invoke(b,band.equals("5")?2:1);builder.getMethod("setAutoShutdownEnabled",boolean.class).invoke(b,false);
   Object cfg=builder.getMethod("build").invoke(b);
   if(args[0].equals("validate")){System.out.println("valid");return;}
   if(!Boolean.TRUE.equals(manager.getMethod("startTetheredHotspot",config,String.class).invoke(wifi,cfg,"com.android.shell")))throw new IllegalStateException("Wi-Fi start rejected");
   System.out.println("started");return;
  }
  if(args[0].equals("untether")&&args.length==2&&args[1].matches("ap[0-9]+")){
   Object tether=service("tethering","android.net.ITetheringConnector");Class<?> connector=Class.forName("android.net.ITetheringConnector"),listener=Class.forName("android.net.IIntResultListener");
   CountDownLatch latch=new CountDownLatch(1);int[] result={-1};
   Binder binder=new Binder(){protected boolean onTransact(int code,Parcel data,Parcel reply,int flags)throws RemoteException{
    if(code==IBinder.INTERFACE_TRANSACTION){reply.writeString("android.net.IIntResultListener");return true;}
    if(code==IBinder.FIRST_CALL_TRANSACTION){data.enforceInterface("android.net.IIntResultListener");result[0]=data.readInt();latch.countDown();return true;}
    return super.onTransact(code,data,reply,flags);
   }};
   Object callback=Proxy.newProxyInstance(listener.getClassLoader(),new Class[]{listener},(o,m,a)->m.getName().equals("asBinder")?binder:null);
   connector.getMethod("untether",String.class,String.class,String.class,listener).invoke(tether,args[1],"com.android.shell",null,callback);
   if(!latch.await(5,TimeUnit.SECONDS)||result[0]!=0)throw new IllegalStateException("Unable to release Android IP service: "+result[0]);System.out.println("released");return;
  }
  throw new IllegalArgumentException("Unsupported operation");
 }
}
