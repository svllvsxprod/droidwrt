package dev.droidwrt.panel;
import java.util.*;
/** Monotonic counter sampling. No estimate is displayed across gaps or source changes. */
public final class DashboardMetrics {
 public double down=-1,up=-1,cpu=-1;
 private long previousTime=-1,rx=-1,tx=-1,total=-1,idle=-1;
 private String source="";private final ArrayList<double[]> points=new ArrayList<>();
 public void reset(){previousTime=-1;rx=tx=total=idle=-1;down=up=cpu=-1;points.clear();}
 public void sample(String name,long now,long received,long sent,long cpuTotal,long cpuIdle){
  long dt=now-previousTime;boolean contiguous=previousTime>=0&&dt>0&&dt<=7000;
  cpu=-1;if(contiguous&&total>=0&&idle>=0&&cpuTotal>total&&cpuIdle>=idle){long all=cpuTotal-total,rest=cpuIdle-idle;if(rest<=all)cpu=100.0*(all-rest)/all;}
  down=up=-1;
  if(!name.equals(source)||!contiguous)points.clear();
  if(contiguous&&name.equals(source)&&!name.isEmpty()&&rx>=0&&tx>=0&&received>=rx&&sent>=tx){down=(received-rx)*8000.0/dt/1000000;up=(sent-tx)*8000.0/dt/1000000;points.add(new double[]{now,down,up});}
  while(!points.isEmpty()&&points.get(0)[0]<now-60000)points.remove(0);
  previousTime=now;source=name;rx=received;tx=sent;total=cpuTotal;idle=cpuIdle;
 }
 public double[][] history(){return points.toArray(new double[0][]);}
 public static String rate(double mbps){if(mbps<0)return "—";if(mbps>0&&mbps<1)return String.format(Locale.getDefault(),"%.1f Кбит/с",mbps*1000);return String.format(Locale.getDefault(),mbps>=100?"%.0f Мбит/с":"%.2f Мбит/с",mbps);}
}
