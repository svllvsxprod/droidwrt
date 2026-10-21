package dev.droidwrt.panel;

import android.os.SystemClock;
import android.graphics.Typeface;
import android.widget.*;
import org.json.*;
import java.util.*;

/** One live report, shared by the dashboard and detailed DNS results. */
final class LeakCheckView {
 final MainActivity a;
 TextView title, subtitle, checks, addresses;
 LinearLayout resolvers;
 Button retry;
 String rendered="", error="";
 boolean requesting;
 LeakCheckView(MainActivity activity){a=activity;}
 JSONObject report(){JSONObject r=a.status.optJSONObject("leaks");return r==null?new JSONObject():r;}
 String state(){
  if(!a.statusAvailable||SystemClock.elapsedRealtime()-a.lastStatusReceived>=7000)return "offline";
  JSONObject r=report(),route=a.status.optJSONObject("router");
  if(route==null)return "pending";
  String mode=route.optString("primary");
  if(mode.equals("direct"))return "direct";
  if(mode.equals("blocked"))return "vpn_unavailable";
  if(!mode.equals(r.optString("mode"))||!route.optString(mode+"_profile").equals(r.optString("profile")))return "pending";
  if(!route.optString(mode+"_state").equals("connected"))return "vpn_unavailable";
  String state=r.optString("state","pending");
  long age=System.currentTimeMillis()/1000-r.optLong("checked_at");
  if(!state.equals("checking")&&(age<0||age>360))return "stale";
  return state;
 }
 String heading(String s){
  switch(s){
   case "passed":return "DNS и IP · проверено";
   case "checking":return "Проверяем DNS и IP…";
   case "mismatch":return "Выход не совпадает с VPN";
   case "incomplete":return "Проверка завершена не полностью";
   case "direct":return "Прямое подключение · без VPN";
   case "offline":return "Нет связи со службой";
   case "vpn_unavailable":return "VPN не подтверждён";
   case "stale":return "Пора обновить проверку";
   default:return "Проверка готовится…";
  }
 }
 int color(String s){return s.equals("passed")?MainActivity.ACCENT:s.equals("mismatch")?(a.lightTheme?0xffa93432:0xffffa49e):MainActivity.MUTED;}
 String time(){long stamp=report().optLong("checked_at");return stamp==0?"":new java.text.SimpleDateFormat("HH:mm",Locale.getDefault()).format(new Date(stamp*1000));}
 int count(){JSONObject p=report().optJSONObject("probe");JSONArray list=p==null?null:p.optJSONArray("resolvers");return list==null?0:list.length();}
 void dashboard(TextView view){String s=state();String detail=s.equals("passed")?"Утечек в тесте не обнаружено · "+time():s.equals("checking")?"Тест маршрута раздачи · до 1 минуты":s.equals("direct")?"Защита VPN выключена":s.equals("mismatch")?"Откройте результаты проверки":"Открыть результаты и DNS-серверы";view.setText(heading(s)+"\n"+detail+"  ›");view.setTextColor(color(s));view.setContentDescription(view.getText());}
 void show(){
  a.startPage("leaks","Проверка защиты",()->a.showHome());
  title=a.text("",25,MainActivity.FG);title.setTypeface(Typeface.create("sans-serif-medium",0));a.body.addView(title);a.gap(a.body,8);
  subtitle=a.text("",13,MainActivity.MUTED);a.body.addView(subtitle);a.gap(a.body,18);
  checks=a.text("",14,MainActivity.FG);checks.setPadding(a.dp(16),a.dp(14),a.dp(16),a.dp(14));checks.setBackground(a.background(MainActivity.SURFACE,14));a.body.addView(checks);a.gap(a.body,12);
  addresses=a.text("",12,MainActivity.MUTED);a.body.addView(addresses);a.gap(a.body,16);
  retry=a.button("Проверить снова",()->retest());
  a.body.addView(a.text("DNS-серверы",19,MainActivity.FG));a.gap(a.body,8);
  resolvers=a.column();a.body.addView(resolvers);a.gap(a.body,16);
  a.note("Тестовый клиент OpenWrt проверяет маршрут раздачи: IPv4, IPv6 и DNS по UDP/TCP. 50 уникальных имён показывают DNS-серверы, обработавшие запросы, как на BrowserLeaks.");
  a.note("Автоматически — каждые 5 минут и при смене маршрута. Страна DNS-сервера сама по себе не означает утечку. Тест не проверяет WebRTC и настройки браузеров подключённых устройств.");
  a.body.addView(a.text("DNS-измерения: browserleaks.com · проверка выхода: Cloudflare",11,MainActivity.MUTED));
  rendered="";update();a.refreshStatus();
 }
 String mark(JSONObject c,String key){String s=c==null?"unknown":c.optString(key);return s.equals("passed")?"✓":s.equals("mismatch")?"!":"—";}
 String label(JSONObject c,String key){String s=c==null?"unknown":c.optString(key);return s.equals("passed")?"через VPN":s.equals("mismatch")?"выход отличается":"не подтверждено";}
 void update(){
  String s=state();JSONObject r=report(),p=r.optJSONObject("probe"),c=r.optJSONObject("checks");
  boolean usable=s.equals("passed")||s.equals("incomplete")||s.equals("mismatch");
  title.setText(heading(s));title.setTextColor(color(s));
  String mode=r.optString("mode");
  subtitle.setText(!error.isEmpty()?error:usable?(mode.equals("awg")?"AWG":"Mihomo")+" · "+a.profileName(r.optString("profile"))+"\nПроверено в "+time()+" · внутренний клиент OpenWrt":s.equals("direct")?"Интернет идёт напрямую. Выберите VPN для проверки защиты.":s.equals("checking")?"Проверяем новый маршрут. Обычно это занимает до минуты.":"Свежий результат пока недоступен.");
  checks.setText(usable?mark(c,"ipv4")+"  IPv4 · "+label(c,"ipv4")+"\n"+mark(c,"ipv6")+"  IPv6 · "+label(c,"ipv6")+"\n"+mark(c,"dns")+"  DNS · "+label(c,"dns")+"\n"+mark(c,"resolvers")+"  Ответы DNS · "+(p==null?0:p.optInt("completed"))+" / 50":"IPv4 · —\nIPv6 · —\nDNS · —\nОтветы DNS · — / 50");
  addresses.setText(usable&&p!=null?"Выход тестового клиента\n"+p.optString("ipv4","—")+"\n"+p.optString("ipv6","—"):"Проверяется трафик за маршрутизатором OpenWrt.");
  retry.setEnabled(!requesting&&!s.equals("checking")&&!s.equals("offline")&&!s.equals("direct")&&!s.equals("vpn_unavailable"));retry.setAlpha(retry.isEnabled()?1:.45f);retry.setText(s.equals("checking")||requesting?"Проверяем…":"Проверить снова");
  JSONArray list=usable&&p!=null?p.optJSONArray("resolvers"):null;String signature=list==null?s:list.toString();if(signature.equals(rendered))return;rendered=signature;
  resolvers.removeAllViews();
  if(list==null||list.length()==0){resolvers.addView(a.text(s.equals("checking")?"Получаем IP, страны и провайдеров…":"Нет подтверждённых данных DNS-серверов",13,MainActivity.MUTED));return;}
  resolvers.addView(a.text("Обнаружено: "+list.length(),12,MainActivity.MUTED));a.gap(resolvers,10);
  for(int i=0;i<list.length();i++){JSONObject item=list.optJSONObject(i);if(item==null)continue;LinearLayout card=a.column();card.setPadding(a.dp(14),a.dp(12),a.dp(14),a.dp(12));card.setBackground(a.background(MainActivity.SURFACE,12));card.addView(a.text(a.flag(item.optString("country"))+"  "+item.optString("ip"),15,MainActivity.FG));a.gap(card,4);card.addView(a.text(item.optString("provider"),13,MainActivity.FG));card.addView(a.text(item.optString("location"),12,MainActivity.MUTED));resolvers.addView(card);a.gap(resolvers,8);}
 }
 void retest(){if(requesting)return;requesting=true;error="";update();a.worker.execute(()->{try{a.api("/droidwrt/leaks",new JSONObject());a.runOnUiThread(()->{requesting=false;if(a.page.equals("leaks"))update();a.refreshStatus();});}catch(Exception e){a.runOnUiThread(()->{requesting=false;error="Не удалось запустить проверку. Попробуйте ещё раз.";if(a.page.equals("leaks"))update();});}});}
}
