"""Local frontend acceptance; synthetic API fixtures, never real question data."""
import argparse
import json
import os
import shutil
from urllib.parse import urlsplit
from playwright.sync_api import sync_playwright, expect

parser=argparse.ArgumentParser()
parser.add_argument('--base-url',default='http://127.0.0.1:5206')
base=parser.parse_args().base_url.rstrip('/')
if urlsplit(base).hostname not in ('127.0.0.1','localhost'):
    raise SystemExit('Only local frontend test servers are accepted')

def question(key,answer=-1,issue=''):
    return {'ID':key,'Kind':'question','IsPublic':True,'PointID':'point','Title':key,'Status':'unavailable','Revision':1,
            'Payload':{'Stem':'虚拟内存采用什么装入方式？','Options':['按需','全部常驻'],'Answer':answer,'AnswerOrigin':'source' if answer>=0 else 'ai_pending',
                       'ReviewStatus':'pending','Issue':issue,'Source':'合成教材第1页','Explanation':'按需装入。'}}
assets=[question('可核对的题目',0),question('缺少答案的题目'),question('需要修复的题目',0,'原文图片缺失')]
errors=[];published=[]
with sync_playwright() as p:
    browser=p.chromium.launch(executable_path=os.environ.get('CHROMIUM_PATH') or shutil.which('chromium'),headless=True,args=['--no-sandbox'])
    page=browser.new_page(viewport={'width':1440,'height':1000})
    page.add_init_script("localStorage.setItem('exam408_token','synthetic-token')")
    page.on('pageerror',lambda e:errors.append(str(e)))
    page.on('dialog',lambda dialog:dialog.accept())
    def respond(route):
        path=urlsplit(route.request.url).path;body=[]
        if path.endswith('/auth/me'):body={'id':'admin','username':'teacher','email':'teacher@example.test','role':'admin'}
        elif path.endswith('/exam408/assets'):body=assets
        elif route.request.method=='PUT' and '/exam408/assets/' in path:
            value=route.request.post_data_json
            assert value['ID']=='可核对的题目'
            assert value['Payload']['ReviewStatus']=='published'
            assert value['Payload']['AnswerOrigin']=='source'
            value['Status']='ready';value['Revision']+=1
            assets[0]=value;published.append(value);body=value
        elif path.endswith('/knowledge-points'):body=[{'ID':'point','Name':'虚拟内存','SubjectName':'操作系统'}]
        route.fulfill(status=200,content_type='application/json',body=json.dumps(body))
    page.route('**/api/v1/**',respond)
    page.goto(base+'/exam408/admin/assets')
    page.get_by_role('heading',name='题库处理进度').wait_for()
    good=page.get_by_label('选择审核发布：可核对的题目',exact=True)
    expect(good).to_be_enabled()
    expect(page.get_by_label('选择审核发布：缺少答案的题目',exact=True)).to_be_disabled()
    good.check()
    page.get_by_role('button',name='发布选中且已核对的题目（1）',exact=True).click()
    page.get_by_role('status').filter(has_text='已发布1道题').wait_for()
    assert len(published)==1
    assert not errors,errors
    browser.close()
print('PASS: review counts, missing-answer protection, explicit reviewed publication; no JS errors')
