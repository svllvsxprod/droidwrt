package dev.droidwrt.panel;
import android.content.Context;import android.graphics.*;import android.view.View;
final class TrafficGraph extends View {
 private double[][] points=new double[0][];private boolean fresh;private final Paint pen=new Paint(Paint.ANTI_ALIAS_FLAG);
 TrafficGraph(Context c){super(c);setContentDescription("График приёма и отдачи за последнюю минуту");}
 void setData(double[][] p,boolean current){points=p;fresh=current;invalidate();}
 @Override protected void onDraw(Canvas c){super.onDraw(c);float w=getWidth(),h=getHeight(),d=getResources().getDisplayMetrics().density;pen.setStrokeWidth(d);pen.setStyle(Paint.Style.STROKE);pen.setColor(MainActivity.LINE);for(int i=0;i<3;i++)c.drawLine(0,5*d+(h-10*d)*i/2,w,5*d+(h-10*d)*i/2,pen);
  if(!fresh||points.length<2){pen.setStyle(Paint.Style.FILL);pen.setColor(MainActivity.MUTED);pen.setTextSize(11*getResources().getDisplayMetrics().scaledDensity);c.drawText(fresh?"Собираем данные…":"Нет свежих данных",0,h/2-7*d,pen);return;}
  double maximum=.05;for(double[] p:points)maximum=Math.max(maximum,Math.max(p[1],p[2]));double end=points[points.length-1][0];
  for(int series=1;series<=2;series++){int n=points.length;double[] x=new double[n],y=new double[n];for(int i=0;i<n;i++){x[i]=w*(1-(end-points[i][0])/60000);y[i]=h-5*d-(h-10*d)*points[i][series]/maximum;}double[] slopes=MonotoneCurve.slopes(x,y);Path path=new Path();path.moveTo((float)x[0],(float)y[0]);for(int i=0;i<n-1;i++){double dx=(x[i+1]-x[i])/3;path.cubicTo((float)(x[i]+dx),(float)(y[i]+slopes[i]*dx),(float)(x[i+1]-dx),(float)(y[i+1]-slopes[i+1]*dx),(float)x[i+1],(float)y[i+1]);}pen.setStyle(Paint.Style.STROKE);pen.setStrokeCap(Paint.Cap.ROUND);pen.setStrokeJoin(Paint.Join.ROUND);pen.setStrokeWidth(2*d);pen.setColor(series==1?MainActivity.ACCENT:MainActivity.BLUE);c.drawPath(path,pen);}
 }
}
