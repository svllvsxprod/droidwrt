package dev.droidwrt.panel;
public class DashboardMetricsTest {
 static void check(boolean value,String why){if(!value)throw new AssertionError(why);}
 public static void main(String[] args){DashboardMetrics m=new DashboardMetrics();m.sample("wan",1000,100,100,1000,500);check(m.down<0,"baseline");m.sample("wan",3000,1000100,250100,1200,550);check(m.down==4&&m.up==1&&m.cpu==75,"rates and CPU");m.sample("wifi",5000,9000000,9000000,1400,600);check(m.down<0&&m.history().length==0,"source change");m.sample("wifi",7000,1,1,1600,650);check(m.down<0,"counter reset");m.sample("wifi",19000,100,100,1800,700);check(m.down<0&&m.cpu<0,"long gap");m.sample("wifi",21000,100,100,2000,750);check(m.down==0&&m.up==0,"idle zero");m.reset();check(m.down<0&&m.history().length==0,"disconnect");System.out.println("Dashboard counter tests PASS");}
}
