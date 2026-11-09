"""Knowledge lesson and answered-example UI regression. Uses isolated API fixtures."""
import os,json
from playwright.sync_api import sync_playwright
origin=os.environ.get('PAGE_TEST_ORIGIN','http://127.0.0.1:5179')
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path=os.environ.get('CHROMIUM_PATH','/usr/bin/chromium'),headless=True,args=['--no-sandbox'])
 page=browser.new_page(viewport={'width':1920,'height':1000});errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 source=dict(DocumentID='doc',DocumentTitle='合成教材',ChunkID='c',Location='第3页',Page=3,Excerpt='分块内部标记不能直接展示',Score=1)
 question=dict(ID='example',Revision=1,Kind='question',PointID='p',Title='合成例题',Status='ready',Payload=dict(Stem='资源分配应当核对什么？',Options=['定义与条件','忽略条件'],Answer=-1,Explanation='',Source='合成教材',Images=[]))
 unit=dict(PointID='p',Name='进程与线程',Goal='学会按条件区分概念。',Reason='有效错题后安排复习。',Date='2026-11-09',ScheduledMinutes=25,EstimatedMinutes=25,Knowledge=[source],Materials=[],Example=question,Practice=[],Steps=[dict(Key=k,Title=t,Minutes=5,ResourceKey='key',Available=True,Completed=False,Notice='') for k,t in [('knowledge','知识复习'),('example','典型例题'),('practice','独立练习'),('check','新题复测')]],CheckAssessmentID='')
 cache=[None];generated=[];attempt=[None];fail_lesson=[False];fail_feedback=[False]
 def respond(route):
  path=route.request.url.split('/api/v1')[-1].split('?')[0];method=route.request.method
  if path=='/auth/me':data=dict(id='u',username='同学',email='test@example.invalid',role='student')
  elif path=='/exam408/tasks/plan':data=dict(ID='plan',Date=unit['Date'],BudgetMinutes=60,AllocatedMinutes=25,Strategy='408-learning-units-v1')
  elif path=='/exam408/assessments':data=[]
  elif path=='/exam408/tasks':data=[]
  elif path=='/exam408/learning-units':data=[unit]
  elif path.endswith('/lesson'):
   if method=='POST':
    if fail_lesson[0]:route.fulfill(status=503,json={'error':'fixture lesson unavailable'});return
    generated.append(1);cache[0]=dict(Answer='本节完整讲解：先明确概念和条件，再解释原理与逻辑，最后核对易混点。',Sources=[source],KnowledgePoints=[],ConversationID='lesson',Generation=dict(Verified=True))
   data={'Lesson':cache[0]}
  elif path.endswith('/example-attempts'):
   if method=='POST':attempt[0]=dict(ID='example-round',Kind='practice',Questions=[question],Result=None,CreatedAt='2026-11-09');data=attempt[0]
   else:data={'Assessment':attempt[0]}
  elif path.endswith('/submit'):
   selected=route.request.post_data_json['Answers']['example'];result=dict(Total=1,Counted=1,Correct=int(selected==0),Items=[dict(ID='example',PointID='p',Selected=selected,Answer=0,Correct=selected==0,Explanation='核对定义与条件后才能判断，所以应选择定义与条件。',AnswerOrigin='source',Counted=True)])
   attempt[0]['Result']=result;unit['Steps'][1]['Completed']=True;data=result
  elif path.endswith('/explain'):
   if fail_feedback[0]:route.fulfill(status=503,json={'error':'fixture teacher unavailable'});return
   data=dict(Answer='老师逐步讲解：先找题目条件，再用定义判断，最后得到正确选项。',Sources=[source],ConversationID='feedback',KnowledgePoints=[])
  elif path=='/exam408/learning/overview':data=dict(Points=[],Memories=[],NeedsPretest=False,Notice='')
  else:data=[]
  route.fulfill(status=200,json=data)
 page.route('**/api/v1/**',respond);page.goto(origin+'/login');page.evaluate("localStorage.setItem('exam408_token','fixture')")
 page.goto(origin+'/exam408/tasks');page.get_by_text('本节完整讲解：先明确概念和条件，再解释原理与逻辑，最后核对易混点。',exact=True).wait_for();assert page.get_by_text('分块内部标记不能直接展示',exact=False).count()==0
 example=page.get_by_role('region',name='例题作答与讲解');assert example.get_by_text('参考答案',exact=False).count()==0
 example.get_by_role('radio').nth(1).check();example.get_by_role('button',name='提交例题并看讲解',exact=True).click();example.get_by_text('这道例题需要复查，已记录到你的学情。',exact=True).wait_for();example.get_by_text('老师逐步讲解：先找题目条件，再用定义判断，最后得到正确选项。',exact=True).wait_for()
 assert attempt[0]['Result']['Items'][0]['Selected']==1
 page.reload();page.get_by_text('本节完整讲解：先明确概念和条件，再解释原理与逻辑，最后核对易混点。',exact=True).wait_for();assert len(generated)==1
 widths=[]
 for path in ['tasks','assessment','weakness']:
  page.goto(origin+'/exam408/'+path);page.locator('main').wait_for();widths.append(page.locator('main').evaluate('(e)=>e.getBoundingClientRect().width'))
 assert max(widths)-min(widths)<1 and max(widths)<=1280,widths
 cache[0]=None;fail_lesson[0]=True;page.goto(origin+'/exam408/tasks');page.get_by_role('alert').filter(has_text='知识讲解暂未完成').wait_for();assert page.get_by_text('分块内部标记不能直接展示',exact=False).count()==0
 for width in [768,390,320]:
  page.set_viewport_size({'width':width,'height':900});page.goto(origin+'/exam408/tasks');page.get_by_role('region',name='例题作答与讲解').wait_for();assert not page.evaluate('document.documentElement.scrollWidth>innerWidth+2'),width
 assert not errors,errors
 print(json.dumps({'widths_equal':widths,'coherent_content_not_raw_chunks':True,'lesson_reuses_cache':True,'example_has_real_submission_then_feedback':True,'failed_lesson_does_not_show_raw_blocks':True,'mobile_no_overflow':True,'js_errors':errors}))
 browser.close()
