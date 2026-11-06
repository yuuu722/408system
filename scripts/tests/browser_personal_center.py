import argparse
import os
import shutil
import json
from urllib.parse import urlsplit
from playwright.sync_api import sync_playwright, expect
args=argparse.ArgumentParser(description='Personal-center browser acceptance with synthetic API fixtures')
args.add_argument('--base-url',default='http://127.0.0.1:5205')
base=args.parse_args().base_url.rstrip('/')
if urlsplit(base).hostname not in ('127.0.0.1','localhost'):
 raise SystemExit('Only local frontend test servers are accepted')
profile={'DisplayName':'旧昵称','Bio':'复习操作系统','GoalScore':110,'DailyMinutes':60,'ExamDate':'','Stage':'review','Preference':'mixed'}
summary={'Assessments':1,'Submitted':1,'Unsubmitted':0,'UncountedAttempts':1,'ValidFamilies':3,'CorrectFamilies':2,'CoveredPoints':2,'TotalPoints':20,'Accuracy':2/3,'Notice':'正确率按题族最近有效作答计算，不等于考试成绩预测。','Subjects':[{'ID':'os','Name':'操作系统','Tested':3,'Correct':2,'CoveredPoints':2,'TotalPoints':20,'Accuracy':2/3},{'ID':'cn','Name':'计算机网络','Tested':0,'Correct':0,'CoveredPoints':0,'TotalPoints':20,'Accuracy':None}],'Recent':[{'ID':'attempt1','Kind':'diagnostic','CreatedAt':'2026-11-06T12:00:00Z','SubmittedAt':'2026-11-06T12:10:00Z','Total':4,'Counted':3,'Correct':2}]}
fail=False;saves=[];errors=[]
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path=os.environ.get('CHROMIUM_PATH') or shutil.which('chromium'),headless=True,args=['--no-sandbox'])
 page=browser.new_page(viewport={'width':1440,'height':1000})
 page.add_init_script("localStorage.setItem('exam408_token','synthetic-token')")
 page.on('pageerror',lambda e:errors.append(str(e)))
 def route(r):
  global profile
  path=urlsplit(r.request.url).path;body={};status=200
  if path.endswith('/auth/me'):body={'id':'u','username':'student','email':'student@example.test','role':'user'}
  elif path.endswith('/exam408/profile'):
   if fail:status=500;body={'error':'synthetic failure'}
   elif r.request.method=='PUT':
    profile=r.request.post_data_json;saves.append(profile.copy());body=profile
   else:body=profile
  elif path.endswith('/study-summary'):body=summary
  elif path.endswith('/assistant/status'):body={'mode':'model','enabled':True,'message':'答疑服务已配置'}
  elif path.endswith('/assessments'):body=[{'ID':'attempt1','Kind':'diagnostic','Questions':[],'Result':{'Total':4,'Correct':2,'Counted':3,'Items':[]},'CreatedAt':'2026-11-06T12:00:00Z'}]
  elif path.endswith('/radar'):body=summary['Subjects']
  else:body=[]
  r.fulfill(status=status,content_type='application/json',body=json.dumps(body))
 page.route('**/api/v1/**',route)
 page.goto(base+'/exam408/settings')
 page.get_by_role('heading',name='个人中心',exact=True).wait_for()
 page.get_by_label('昵称',exact=True).fill('复习同学')
 page.get_by_label('个人简介',exact=True).fill('操作系统与数据结构')
 page.get_by_role('button',name='保存个人信息',exact=True).click()
 page.get_by_role('status').filter(has_text='已保存').wait_for()
 assert saves[-1]['DisplayName']=='复习同学'
 assert 'UserID' not in saves[-1]
 assert page.get_by_text('样本较少，暂不判断整体水平',exact=False).count()>0
 assert page.get_by_text('尚无有效作答',exact=False).count()>0
 page.get_by_role('link',name='查看结果',exact=True).click()
 page.wait_for_url('**/assessment?assessment=attempt1')
 page.get_by_role('heading',name='测评与学情').wait_for()
 page.goto(base+'/exam408/settings')
 expect(page.get_by_label('昵称',exact=True)).to_have_value('复习同学')
 page.set_viewport_size({'width':390,'height':844})
 assert page.evaluate('document.documentElement.scrollWidth <= innerWidth + 2')
 fail=True;page.get_by_role('button',name='刷新数据',exact=True).click()
 page.get_by_role('alert').filter(has_text='个人信息读取失败').wait_for()
 assert page.get_by_role('button',name='保存个人信息',exact=True).is_disabled()
 assert not errors,errors
 browser.close()
print('PASS: profile persistence, evidence states, assessment resume link, mobile width, failed-load write protection; no JS errors')
