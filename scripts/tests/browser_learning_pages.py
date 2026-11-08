"""Run against a local Vite server; API responses are isolated fixtures, no user data.
Requires Python Playwright and Chromium. PAGE_TEST_ORIGIN defaults to localhost:5179.
"""
import json,os
origin=os.environ.get("PAGE_TEST_ORIGIN","http://127.0.0.1:5179")
from playwright.sync_api import sync_playwright
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path=os.environ.get('CHROMIUM_PATH','/usr/bin/chromium'),headless=True,args=['--no-sandbox'])
 page=browser.new_page(viewport={'width':1440,'height':1000});errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 def point(id,name,subject):return dict(ID=id,Name=name,Code=id,SubjectID=subject,SubjectName=subject,ParentID='',Description='用于页面回归的知识点',State='待复查（有错题）',Tested=2,Correct=0,AvailableQuestions=20,ExamYears=4,CorpusYears=5,Doubts=0,Accuracy=0,Importance=4.2,ImportanceBasis='库内真题年度覆盖',Priority=73.6,Sources=[])
 points=[point('p1','进程与线程','操作系统'),point('p2','网络层','计算机网络')]
 plan=dict(ID='plan',Date='2026-11-08',BudgetMinutes=60,AllocatedMinutes=10,Strategy='test')
 def task(n,date):return dict(ID='t'+str(n),AssetID='a'+str(n),PointID='p'+str(n),Title='专项复习'+str(n),Kind='question',Reason='根据最近有效作答安排复查。',PlanDate=date,EstimatedMinutes=5,Completed=False)
 tasks=[task(1,plan['Date']),task(2,plan['Date']),task(3,'2026-11-07')];starts=[];fail_plan=[False];fail_generate=[False]
 def respond(route):
  path=route.request.url.split('/api/v1')[-1]
  if path=='/auth/me':data=dict(id='u',username='测试同学',email='fixture@example.invalid',role='student')
  elif path=='/exam408/learning/overview':data=dict(Points=points,Memories=[],NeedsPretest=False,Notice='')
  elif path=='/exam408/tasks/plan':
   if fail_plan[0]:route.fulfill(status=503,json={'error':'fixture plan unavailable'});return
   data=plan
  elif path=='/exam408/tasks/generate':
   if fail_generate[0]:route.fulfill(status=503,json={'error':'fixture generation unavailable'});return
   data={'ok':True}
  elif path=='/exam408/tasks':data=tasks
  elif path.endswith('/submit'):data=dict(Total=1,Counted=1,Correct=1,Items=[dict(ID='a2',PointID='p2',Selected=0,Answer=0,Correct=True,Explanation='样本原文解析',AnswerOrigin='source',Counted=True)])
  elif path=='/exam408/assessments' and route.request.method=='POST':
   body=route.request.post_data_json;starts.append(body['AssetID']);data=dict(ID='round'+body['AssetID'],Kind='practice',Questions=[dict(ID=body['AssetID'],Title=body['AssetID'],Payload=dict(Stem='题干'+body['AssetID'],Options=['甲','乙','丙','丁'],Images=[],Source='回归样本'))],Result=None,CreatedAt='2026-11-08')
  elif path=='/exam408/coaching':data=[dict(ID='old',PointID='p2',Teacher='encouraging',CreatedAt='2026-11-08',Lesson={'Answer':'不应在进程课堂显示的旧网络课堂','Sources':[]},Assessment={'Questions':[],'Result':None})]
  elif path=='/knowledge-bases':data=[]
  elif path=='/exam408/radar':data=[]
  else:data=[]
  route.fulfill(status=200,json=data)
 page.route('**/api/v1/**',respond)
 page.goto(origin+'/login');page.evaluate("localStorage.setItem('exam408_token','fixture')")
 page.goto(origin+'/exam408/tasks?tab=teacher');page.get_by_role('heading',name='今日复习计划',exact=True).wait_for()
 page.get_by_text('专项复习1',exact=True).wait_for()
 assert page.get_by_text('专项复习3',exact=True).count()==0
 assert page.locator('section.lesson').count()==0
 fail_plan[0]=True;page.get_by_role('button',name='更新计划',exact=True).click();page.get_by_role('alert').filter(has_text='计划暂未获取成功').wait_for();assert page.get_by_text('专项复习1',exact=True).count()==0
 fail_plan[0]=False;page.get_by_role('button',name='更新计划',exact=True).click();page.get_by_text('专项复习1',exact=True).wait_for()
 page.get_by_role('link',name='开始练习 →',exact=True).first.click();page.get_by_text('题干a1',exact=True).wait_for()
 def navigate(url):page.evaluate('(url)=>{history.pushState({},"",url);dispatchEvent(new PopStateEvent("popstate"))}',url)
 navigate('/exam408/assessment?asset=a2&point=p2');page.get_by_text('题干a2',exact=True).wait_for();assert page.get_by_text('题干a1',exact=True).count()==0;assert starts==['a1','a2'],starts
 fail_generate[0]=True;page.get_by_role('radio').first.check();page.get_by_role('button',name='提交并查看报告',exact=True).click();page.get_by_text('计入学情 1 / 1 题，其中正确 1 题',exact=True).wait_for();page.get_by_role('status').filter(has_text='计划更新失败').wait_for();fail_generate[0]=False
 navigate('/exam408/tasks');page.get_by_role('heading',name='我的学习任务',exact=True).wait_for();page.get_by_role('heading',name='专项复习1 待学习',exact=True).wait_for();assert page.get_by_text('专项复习3',exact=True).count()==0
 nav=page.get_by_role('navigation',name='主导航')
 assert nav.get_by_role('link').count()==9
 for label,path,title in [('前测练习与报告','assessment','前测、练习与报告'),('薄弱点复习','weakness','薄弱点复习'),('学情统计','statistics','学情统计')]:
  nav.get_by_role('link',name=label,exact=True).click();page.wait_for_url('**/exam408/'+path);page.get_by_role('heading',name=title,exact=True).wait_for()
  assert page.get_by_role('navigation',name='测评学情分区').count()==0
  assert nav.get_by_role('link',name=label,exact=True).get_attribute('aria-current')=='page'
 navigate('/exam408/assessment?tab=weakness&point=p1');page.wait_for_url('**/exam408/weakness?point=p1')
 navigate('/exam408/assessment?tab=statistics&point=p2');page.wait_for_url('**/exam408/statistics?point=p2')
 page.set_viewport_size({'width':1920,'height':1080});navigate('/exam408/assessment');page.get_by_role('heading',name='前测、练习与报告',exact=True).wait_for()
 assert page.locator('main').evaluate('(e)=>e.getBoundingClientRect().width')>1800
 page.screenshot(path='/tmp/assessment-independent-desktop.png',full_page=True)
 for width in [1440,1024,768,390,320]:
  page.set_viewport_size({'width':width,'height':900});navigate('/exam408/tasks?tab=teacher');page.get_by_role('heading',name='我的老师与学习方案',exact=True).wait_for();page.wait_for_timeout(100)
  assert not page.evaluate('document.documentElement.scrollWidth>innerWidth+2'),('overflow',width)
  assert page.locator('.memory-form select').evaluate('(e)=>e.getBoundingClientRect().width')>100
 assert not errors,errors
 print(json.dumps({'same_page_asset_switch':starts,'historical_tasks_excluded':True,'responsive_widths':[1440,1024,768,390,320],'js_errors':errors}))
 browser.close()
