"""Isolated point-study UI fixtures. Requires local Vite, Playwright and Chromium."""
import json,os
from playwright.sync_api import sync_playwright
origin=os.environ.get('PAGE_TEST_ORIGIN','http://127.0.0.1:5179')
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path=os.environ.get('CHROMIUM_PATH','/usr/bin/chromium'),headless=True,args=['--no-sandbox'])
 page=browser.new_page(viewport={'width':1440,'height':1000});errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 def point(id,name,parent,tested,correct,available,subtree,doubts=0):return dict(ID=id,Name=name,Code=id,ParentID=parent,SubjectID='os',SubjectName='操作系统',Description='目录范围，非教材讲解。',State='待复查（有错题）' if tested>correct else '待测',Tested=tested,Correct=correct,AvailableQuestions=available,SubtreeQuestions=subtree,ExamYears=1,CorpusYears=2,Doubts=doubts,Accuracy=correct/tested if tested else None,Importance=3,ImportanceBasis='合成年度覆盖样本',Priority=50,Sources=[])
 points=[point('root','操作系统基础','',0,0,0,2),point('p1','进程与线程','root',2,0,2,2),point('p2','虚拟内存','root',0,0,0,0,1)]
 def question(id):return dict(ID=id,Revision=1,Kind='question',PointID='p1',Title='合成题'+id,Status='ready',Payload=dict(Stem='进程和线程如何区分？',Options=['按资源与调度区分','完全相同'],Answer=0,Explanation='应从资源分配和调度单位等方面区分。',Source='合成教材题目',Images=[]))
 def pack(id):
  pt=next(q for q in points if q['ID']==id);ex=question('example') if id!='p2' else None;q=question('practice');q['Payload'].update(Answer=-1,Explanation='')
  return dict(Point=pt,Ancestors=[] if id=='root' else [points[0]],Children=points[1:] if id=='root' else [],DirectQuestions=pt['AvailableQuestions'],SubtreeQuestions=pt['SubtreeQuestions'],Knowledge=[] if id=='p2' else [dict(DocumentID='doc',DocumentTitle='合成教材',ChunkID='c'+id,Location='第3页',Page=3,Excerpt='教材定义：'+pt['Name']+'需要结合适用条件理解。',Score=1)],Example=ex,Exercises=[q] if id!='p2' else [],Mistakes=[dict(AssessmentID='wrong-round',Question=question('wrong'),Result=dict(Selected=-1,Answer=0,Explanation='核对资源与调度的区别。'),SubmittedAt='2026-11-09')] if id=='p1' else [],Missing=['尚未定位到可核对教材段落，不能把目录描述当成完整讲解。'] if id=='p2' else [])
 failed=[False];delay=[False];held=[]
 def respond(route):
  path=route.request.url.split('/api/v1')[-1]
  if path=='/auth/me':data=dict(id='u',username='回归同学',email='ui@example.invalid',role='student')
  elif path=='/exam408/learning/overview':data=dict(Points=points,Memories=[],NeedsPretest=False,Notice='')
  elif path.endswith('/study'):
   id=path.split('/')[-2]
   if delay[0] and id=='p1':held.append(route);return
   data=pack(id)
  elif path.endswith('/example-attempts'):data={'Assessment':None}
  elif path=='/knowledge-bases':data=[dict(ID='public',Name='公共教材',IsPublic=True)]
  elif path=='/exam408/assistant/chat':
   if failed[0]:route.fulfill(status=503,json={'error':'fixture model unavailable'});return
   data=dict(Answer='老师讲解样本：先区分资源分配和调度，再核对适用条件。',Sources=pack('p1')['Knowledge'],KnowledgePoints=[],ConversationID='inline-lesson')
  else:data=[]
  route.fulfill(status=200,json=data)
 page.route('**/api/v1/**',respond)
 page.goto(origin+'/login');page.evaluate("localStorage.setItem('exam408_token','fixture')")
 page.goto(origin+'/exam408/weakness');page.get_by_text('教材定义：进程与线程需要结合适用条件理解。',exact=True).wait_for()
 assert page.locator('.review-list>button').count()==1;assert page.get_by_text('合成题wrong',exact=True).count()==1
 page.get_by_role('button',name='请老师在这里讲解',exact=True).click();page.get_by_text('老师讲解样本：先区分资源分配和调度，再核对适用条件。',exact=True).wait_for();assert page.get_by_role('button',name='▶ 语音讲解',exact=True).count()==1
 failed[0]=True;page.get_by_role('button',name='请老师在这里讲解',exact=True).click();page.get_by_role('alert').filter(has_text='老师讲解暂未完成').wait_for();assert page.get_by_text('教材定义：进程与线程需要结合适用条件理解。',exact=True).count()==1;failed[0]=False
 page.goto(origin+'/exam408/knowledge?point=root');page.get_by_text('教材定义：操作系统基础需要结合适用条件理解。',exact=True).wait_for();assert page.locator('.map-tree').get_by_text('待测 · 含下级2个题组',exact=True).count()==1
 page.locator('.map-tree button').filter(has_text='进程与线程').click();page.get_by_text('教材定义：进程与线程需要结合适用条件理解。',exact=True).wait_for()
 page.locator('.map-tree button').filter(has_text='虚拟内存').click();page.get_by_text('暂无有可靠答案与解析的参考例题。',exact=False).wait_for()
 delay[0]=True;page.locator('.map-tree button').filter(has_text='进程与线程').click();page.wait_for_timeout(100);page.locator('.map-tree button').filter(has_text='虚拟内存').click();page.get_by_role('heading',name='虚拟内存',exact=True).wait_for()
 for route in held:route.fulfill(status=200,json=pack('p1'))
 page.wait_for_timeout(100);assert page.get_by_text('教材定义：进程与线程需要结合适用条件理解。',exact=True).count()==0;delay[0]=False
 for width in [1440,768,390,320]:
  page.set_viewport_size({'width':width,'height':900})
  for path in ['weakness','knowledge']:
   page.goto(origin+'/exam408/'+path+'?point=p1');page.get_by_text('教材定义：进程与线程需要结合适用条件理解。',exact=True).wait_for();assert not page.evaluate('document.documentElement.scrollWidth>innerWidth+2'),(path,width)
 assert not errors,errors
 print(json.dumps({'weakness_has_sources_mistakes_examples_exercises':True,'doubts_not_mistakes':True,'inline_teaching_and_voice':True,'teacher_failure_keeps_textbook':True,'parent_has_descendant_coverage':True,'stale_response_ignored':True,'mobile_no_overflow':True,'js_errors':errors}))
 browser.close()
