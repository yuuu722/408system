export function rotateBox(box:number[],degrees:number):number[]{
 const [x0=0,y0=0,x1=0,y1=0]=box
 switch(((degrees%360)+360)%360){case 90:return [1-y1,x0,1-y0,x1];case 180:return [1-x1,1-y1,1-x0,1-y0];case 270:return [y0,1-x1,y1,1-x0];default:return [x0,y0,x1,y1]}
}
export function validBox(box:number[]|undefined):box is number[]{return !!box&&box.length===4&&box.every(n=>Number.isFinite(n)&&n>=0&&n<=1)&&box[2]!>box[0]!&&box[3]!>box[1]!}
