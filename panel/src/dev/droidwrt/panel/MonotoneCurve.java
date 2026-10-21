package dev.droidwrt.panel;
/** Cubic tangents preserving every measured sample without overshooting a segment. */
final class MonotoneCurve {
 static double[] slopes(double[] x,double[] y){int n=x.length;double[] m=new double[n];if(n<2)return m;double[] d=new double[n-1];for(int i=0;i<n-1;i++)d[i]=(y[i+1]-y[i])/(x[i+1]-x[i]);m[0]=d[0];m[n-1]=d[n-2];for(int i=1;i<n-1;i++)m[i]=d[i-1]*d[i]<=0?0:2/(1/d[i-1]+1/d[i]);for(int i=0;i<n-1;i++){if(d[i]==0){m[i]=m[i+1]=0;continue;}double a=m[i]/d[i],b=m[i+1]/d[i],norm=Math.hypot(a,b);if(norm>3){double scale=3/norm;m[i]=scale*a*d[i];m[i+1]=scale*b*d[i];}}return m;}
}
