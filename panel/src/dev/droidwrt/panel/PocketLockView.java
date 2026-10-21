package dev.droidwrt.panel;

import android.graphics.*;
import android.os.*;
import android.view.*;
import android.view.accessibility.AccessibilityNodeInfo;
import android.widget.*;
import org.json.JSONObject;

/** An accidental-touch guard for the panel, not a credential or system lock. */
final class PocketLockView extends FrameLayout {
 final MainActivity owner; final TextView connection; final TextClock clock; final CoreStatus awg,mihomo;
 PocketLockView(MainActivity a,Runnable unlock){
  super(a); owner=a;setBackgroundColor(MainActivity.BG);setClickable(true);setFocusable(true);
  LinearLayout layout=a.column();layout.setPadding(a.dp(24),a.dp(12),a.dp(24),a.dp(28));addView(layout,new FrameLayout.LayoutParams(-1,-1));
  LinearLayout center=a.column();center.setGravity(Gravity.CENTER);layout.addView(center,new LinearLayout.LayoutParams(-1,0,1));
  TextView brand=a.text("DroidWRT",15,MainActivity.MUTED);brand.setGravity(Gravity.CENTER);brand.setLetterSpacing(.06f);center.addView(brand,new LinearLayout.LayoutParams(-1,-2));a.gap(center,6);
  clock=new TextClock(a);clock.setFormat24Hour("HH:mm");clock.setFormat12Hour("HH:mm");clock.setGravity(Gravity.CENTER);clock.setTextSize(64);clock.setTextColor(MainActivity.FG);clock.setTypeface(Typeface.create("sans-serif-light",0));center.addView(clock,new LinearLayout.LayoutParams(-1,-2));a.gap(center,8);
  connection=a.text("Проверка соединения…",12,MainActivity.MUTED);connection.setGravity(Gravity.CENTER);center.addView(connection,new LinearLayout.LayoutParams(-1,-2));a.gap(center,24);
  awg=new CoreStatus("awg");mihomo=new CoreStatus("mihomo");center.addView(awg,new LinearLayout.LayoutParams(-1,-2));a.gap(center,10);center.addView(mihomo,new LinearLayout.LayoutParams(-1,-2));
  LinearLayout bottom=a.column();bottom.setGravity(Gravity.CENTER_HORIZONTAL);layout.addView(bottom,new LinearLayout.LayoutParams(-1,-2));
  TextView hint=a.text("Защита от случайных касаний",12,MainActivity.MUTED);hint.setGravity(Gravity.CENTER);bottom.addView(hint);a.gap(bottom,14);
  bottom.addView(new SlideToUnlock(a,unlock),new LinearLayout.LayoutParams(a.dp(280),a.dp(56)));
  update();
 }
 static void label(TextView view,String value){if(!value.contentEquals(view.getText()))view.setText(value);}
 final class CoreStatus extends LinearLayout {
  final String engine;final TextView role,state,ip4,ip6;
  CoreStatus(String id){super(owner);engine=id;setOrientation(VERTICAL);setPadding(owner.dp(16),owner.dp(13),owner.dp(16),owner.dp(13));setBackground(owner.background(MainActivity.SURFACE,18));
   LinearLayout identity=new LinearLayout(owner);identity.setGravity(Gravity.CENTER_VERTICAL);identity.addView(owner.icon(owner.engineIcon(id),25));
   TextView name=owner.text(id.equals("awg")?"AWG":"Mihomo",16,MainActivity.FG);name.setTypeface(Typeface.create("sans-serif-medium",0));name.setPadding(owner.dp(9),0,0,0);identity.addView(name,new LayoutParams(0,-2,1));
   role=owner.text("",11,MainActivity.ACCENT);identity.addView(role);addView(identity);owner.gap(this,8);
   state=owner.text("",12,MainActivity.MUTED);addView(state);owner.gap(this,6);
   ip4=owner.text("",13,MainActivity.FG);addView(ip4);ip6=owner.text("",12,MainActivity.MUTED);ip6.setBreakStrategy(android.text.Layout.BREAK_STRATEGY_SIMPLE);addView(ip6);
  }
  String address(JSONObject links,JSONObject r,String key,boolean fresh){
   JSONObject link=links==null?null:links.optJSONObject(key);long age=link==null?Long.MAX_VALUE:System.currentTimeMillis()/1000-link.optLong("checked_at");
   boolean valid=fresh&&link!=null&&link.optString("profile").equals(r.optString(engine+"_profile"))&&age>=0&&age<=90;
   if(!valid)return "Адрес проверяется…";
   if(!link.optString("state").equals("verified"))return link.optString("state").equals("not_configured")?"Адрес не назначен":"Адрес недоступен";
   return owner.flag(link.optString("country"))+"  "+link.optString("ip");
  }
  void update(JSONObject r,JSONObject links,boolean fresh){
   boolean primary=r.optString("primary").equals(engine),connected=fresh&&r.optString(engine+"_state").equals("connected");
   label(role,primary?(r.optBoolean("automatic_reserve")?"Активный":"Основной"):r.optString("preferred").equals(engine)?"Основной":"Резерв");role.setTextColor(primary?MainActivity.ACCENT:MainActivity.MUTED);
   String value=!fresh?"Нет свежих данных":connected?"●  Подключён":r.optString(engine+"_state").equals("degraded")?"●  Нестабильная сеть":r.optString(engine+"_state").equals("unresponsive")?"●  Нет ответа VPN":r.optString(engine+"_profile").isEmpty()?"Не настроен":"●  Подключается…";
   label(state,value);state.setTextColor(connected?MainActivity.ACCENT:MainActivity.MUTED);label(ip4,address(links,r,engine,fresh));label(ip6,address(links,r,engine+"_ipv6",fresh));
  }
 }
 void update(){
  boolean fresh=owner.statusAvailable&&SystemClock.elapsedRealtime()-owner.lastStatusReceived<7000;
  JSONObject r=owner.status.optJSONObject("router");if(r==null)r=new JSONObject();
  String primary=r.optString("primary");
  label(connection,!fresh?"Нет свежих данных":primary.equals("direct")?"Напрямую · без VPN":primary.equals("blocked")?"Трафик заблокирован · восстанавливаем VPN":r.optBoolean("route_ready")?(r.optBoolean("automatic_reserve")?"Резерв · ":"Трафик через ")+(primary.equals("awg")?"AWG":"Mihomo"):"VPN · нет подтверждения связи");
  JSONObject links=owner.status.optJSONObject("links");awg.update(r,links,fresh);mihomo.update(r,links,fresh);
 }
 static final class SlideToUnlock extends View {
  final MainActivity owner;final Runnable unlock;final Paint paint=new Paint(Paint.ANTI_ALIAS_FLAG); final android.graphics.drawable.Drawable lock;
  float progress=0,startX;int pointer=-1;boolean dragging=false,completed=false;long started;
  SlideToUnlock(MainActivity a,Runnable action){super(a);owner=a;unlock=action;lock=a.getDrawable(R.drawable.ic_lock).mutate();lock.setTint(MainActivity.BG);setFocusable(true);setContentDescription("Разблокировать управление: перетащите ползунок вправо");setImportantForAccessibility(IMPORTANT_FOR_ACCESSIBILITY_YES);}
  float inset(){return owner.dp(5);} float knob(){return getHeight()-2*inset();} float travel(){return Math.max(1,getWidth()-2*inset()-knob());}
  @Override protected void onDraw(Canvas c){
   float h=getHeight(),pad=inset(),size=knob();
   paint.setColor(MainActivity.SURFACE);c.drawRoundRect(0,0,getWidth(),h,h/2,h/2,paint);
   paint.setColor(MainActivity.LINE);paint.setStyle(Paint.Style.STROKE);paint.setStrokeWidth(owner.dp(1));c.drawRoundRect(.5f,.5f,getWidth()-.5f,h-.5f,h/2,h/2,paint);paint.setStyle(Paint.Style.FILL);
   paint.setColor(MainActivity.MUTED);paint.setAlpha((int)(255*(1-progress)));paint.setTextSize(owner.dp(13));paint.setTypeface(Typeface.create("sans-serif-medium",0));paint.setTextAlign(Paint.Align.CENTER);
   c.drawText("Проведите вправо",getWidth()/2f+owner.dp(16),h/2-(paint.ascent()+paint.descent())/2,paint);paint.setAlpha(255);
   float x=pad+travel()*progress;paint.setColor(MainActivity.ACCENT);c.drawRoundRect(x,pad,x+size,pad+size,size/2,size/2,paint);
   int cx=Math.round(x+size/2),cy=Math.round(h/2),half=owner.dp(10);lock.setBounds(cx-half,cy-half,cx+half,cy+half);lock.draw(c);
  }
  void reset(){dragging=false;pointer=-1;progress=0;invalidate();}
  @Override public boolean onTouchEvent(android.view.MotionEvent e){
   if(completed)return true;
   switch(e.getActionMasked()){
    case MotionEvent.ACTION_DOWN:
     reset();if(e.getX()>=inset()&&e.getX()<=inset()+knob()&&e.getY()>=0&&e.getY()<=getHeight()){
      dragging=true;pointer=e.getPointerId(0);startX=e.getX();started=e.getEventTime();getParent().requestDisallowInterceptTouchEvent(true);
     }return true;
    case MotionEvent.ACTION_POINTER_DOWN:case MotionEvent.ACTION_CANCEL:reset();return true;
    case MotionEvent.ACTION_MOVE:
     int i=e.findPointerIndex(pointer);if(!dragging||i<0)return true;
     if(e.getY(i)<-owner.dp(24)||e.getY(i)>getHeight()+owner.dp(24)){reset();return true;}
     progress=Math.max(0,Math.min(1,(e.getX(i)-startX)/travel()));invalidate();return true;
    case MotionEvent.ACTION_UP:
     if(dragging&&progress>=.94f&&e.getEventTime()-started>=150){completed=true;performHapticFeedback(HapticFeedbackConstants.CONFIRM);unlock.run();}else reset();return true;
   }
   return true;
  }
  @Override public void onInitializeAccessibilityNodeInfo(AccessibilityNodeInfo info){super.onInitializeAccessibilityNodeInfo(info);info.addAction(new AccessibilityNodeInfo.AccessibilityAction(AccessibilityNodeInfo.ACTION_CLICK,"Разблокировать управление"));}
  @Override public boolean performAccessibilityAction(int action,Bundle args){if(action==AccessibilityNodeInfo.ACTION_CLICK){if(!completed){completed=true;unlock.run();}return true;}return super.performAccessibilityAction(action,args);}
 }
}
