#!/usr/bin/env python3
"""Offline clone of a Compose development environment, including stopped volumes.
The bundle is PRIVATE: it includes accounts, uploaded files and runtime secrets.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time

SERVICES = ('postgres', 'redis', 'neo4j', 'backend', 'worker', 'frontend')
VOLUMES = {'postgres-data': ('postgres', '/var/lib/postgresql/data'),
           'redis-data': ('redis', '/data'), 'neo4j-data': ('neo4j', '/data'),
           'uploads': ('backend', '/data/uploads')}
REMOTE = 'https://github.com/yuuu722/408system.git'


def command(args, capture=True, cwd=None):
    result = subprocess.run([str(a) for a in args], cwd=cwd, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, text=True, encoding='utf-8', errors='replace')
    if result.returncode:
        # No compose config, environment values, or login credentials in errors.
        raise RuntimeError(f'命令失败（{result.returncode}）：{args[0]} {args[1] if len(args)>1 else ""}。请单独运行该命令检查；备份未标记完成。')
    return result.stdout.strip() if capture else None


def json_command(args, cwd=None):
    return json.loads(command(args, cwd=cwd))


def digest(path):
    value = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(8 * 1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()


def model_directory():
    return Path(os.environ.get('OLLAMA_MODELS') or Path.home() / '.ollama' / 'models').expanduser().resolve()


def validate_manifest(manifest):
    if manifest.get('format') != 1 or set(manifest.get('volumes', {})) != set(VOLUMES):
        raise ValueError('备份格式或数据卷清单不完整')
    if set(manifest.get('images', {})) != set(SERVICES):
        raise ValueError('镜像清单不完整')
    for value in manifest['images'].values():
        if not re.fullmatch(r'exam408-transfer/[a-z0-9]+:[a-z0-9-]+', value):
            raise ValueError('镜像名称无效')
    required = {'code.bundle', 'runtime.env', 'images.tar'} | {f'{key}.tgz' for key in VOLUMES}
    checks = manifest.get('sha256', {})
    if not required.issubset(checks):
        raise ValueError('缺少必要文件校验记录')
    for name, checksum in checks.items():
        path = Path(name)
        if path.is_absolute() or '..' in path.parts or '\\' in name or ':' in name or not re.fullmatch(r'[a-f0-9]{64}', checksum):
            raise ValueError('备份校验路径或校验值无效')


def archive_volume(helper, volume, folder, name, restore=False):
    # Images are preloaded; no helper download or network access is required.
    source = f'type=volume,source={volume},target=/volume' + ('' if restore else ',readonly')
    command(['docker', 'run', '--rm', '--network', 'none', '--user', '0', '--entrypoint', 'sh',
             '--mount', source, '--mount', f'type=bind,source={folder},target=/backup', helper,
             '-c', f'tar -xzpf /backup/{name}.tgz -C /volume' if restore else
             f'tar -czpf /backup/{name}.tgz -C /volume .'])


def export_environment(args):
    root = Path(__file__).resolve().parents[1]
    output = Path(args.output).expanduser().resolve()
    if root == output or root in output.parents:
        raise ValueError('备份目录必须放在项目目录外，避免进入Git或递归打包')
    if output.exists():
        raise ValueError('输出目录已经存在，请选择新的空目录名称')
    if args.include_ollama and (output == model_directory() or model_directory() in output.parents):
        raise ValueError('备份目录不能放在Ollama模型目录内')
    info = json_command(['docker', 'info', '--format', '{{json .}}'])
    if info['OSType'] != 'linux':
        raise ValueError('Docker Desktop必须切换为Linux容器')
    if command(['git', 'status', '--porcelain', '--untracked-files=no'], cwd=root):
        raise ValueError('有未提交的跟踪文件修改，请先保存提交；不会打包与源码版本不一致的工作区')
    config = json_command(['docker', 'compose', 'config', '--format', 'json'], cwd=root)
    if set(config.get('services', {})) != set(SERVICES) or set(config.get('volumes', {})) != set(VOLUMES):
        raise ValueError('Compose包含不同的服务或数据卷，本工具不能保证完整迁移，已停止')
    env_path = Path(os.environ.get('APP_ENV_FILE', '.env'))
    if not env_path.is_absolute():
        env_path = root / env_path
    if not env_path.is_file():
        raise ValueError('找不到当前应用环境文件')
    # A distinct APP_ENV_FILE would also require separate Compose interpolation
    # secrets; do not silently package a mismatched set of passwords.
    if env_path.resolve() != (root / '.env').resolve():
        raise ValueError('检测到自定义APP_ENV_FILE；请先统一并核对Compose和应用配置，避免数据库密码不匹配')
    mounts, image_ids, running = {}, {}, []
    for service in SERVICES:
        cid = command(['docker', 'compose', 'ps', '-a', '-q', service], cwd=root)
        if not cid or '\n' in cid:
            raise ValueError(f'{service}必须有且只有一个现有容器')
        container = json_command(['docker', 'inspect', cid])[0]
        expected = config['services'][service].get('environment') or {}
        actual = dict(item.split('=', 1) for item in container['Config'].get('Env', []) if '=' in item)
        if any(value is not None and str(value) != actual.get(key) for key, value in expected.items()):
            raise ValueError(f'{service}运行配置与当前Compose不一致，请先核对；不会输出密钥')
        image_ids[service] = container['Image']
        if container['State']['Running']:
            running.append(service)
        for key, (owner, target) in VOLUMES.items():
            if owner == service:
                match = [m for m in container['Mounts'] if m['Destination'] == target and m['Type'] == 'volume']
                if len(match) != 1:
                    raise ValueError(f'无法确认{key}的实际数据卷')
                mounts[key] = match[0]['Name']
    # Resolve actual mounts from containers, never assume a Compose project prefix.
    if set(mounts) != set(VOLUMES):
        raise ValueError('数据卷不完整')
    output.mkdir(parents=True)
    stamp = time.strftime('%Y%m%d%H%M%S')
    images = {service: f'exam408-transfer/{service}:{stamp}' for service in SERVICES}
    manifest = {'format': 1, 'created': stamp, 'architecture': info['Architecture'],
                'source_commit': command(['git', 'rev-parse', 'HEAD'], cwd=root),
                'images': images, 'volumes': mounts, 'sha256': {}, 'ollama': False}
    # Save heavy immutable images before stopping the website.
    print('导出实际运行的镜像；此阶段网站继续运行。', flush=True)
    for service, tag in images.items():
        command(['docker', 'tag', image_ids[service], tag])
    command(['docker', 'image', 'save', '-o', output / 'images.tar', *images.values()])
    command(['git', 'bundle', 'create', output / 'code.bundle', '--all'], cwd=root)
    shutil.copy2(env_path, output / 'runtime.env')
    shutil.copy2(Path(__file__), output / 'transfer-environment.py')
    if args.include_ollama:
        models = model_directory()
        if not (models / 'manifests').is_dir() or not (models / 'blobs').is_dir():
            raise ValueError('未找到完整Ollama模型目录；备份未完成，不会停止网站')
        print('复制Ollama模型，请勿同时下载或删除模型。', flush=True)
        shutil.copytree(models, output / 'ollama-models')
        manifest['ollama'] = True
    stopped = False
    try:
        print('现在短暂停止全部项目服务，保存四个一致的数据卷。', flush=True)
        # stop is graceful and does not remove containers or volumes.
        stopped = True
        command(['docker', 'compose', 'stop', '--timeout', '120'], cwd=root)
        for service in SERVICES:
            cid = command(['docker', 'compose', 'ps', '-a', '-q', service], cwd=root)
            state = json_command(['docker', 'inspect', cid])[0]['State']
            if state['Running']:
                raise RuntimeError('仍有服务运行，拒绝备份不一致数据')
            if service in ('postgres', 'neo4j', 'redis') and state.get('ExitCode') == 137:
                raise RuntimeError('数据库未正常退出，请先检查服务状态；不导出可能不完整的数据')
        for key, volume in mounts.items():
            print('备份数据卷：' + key, flush=True)
            archive_volume(images['redis'], volume, output, key)
    finally:
        if stopped and running:
            # start existing containers; never rebuild, migrate, or reparse.
            command(['docker', 'compose', 'start', *running], cwd=root)
            print('原电脑的服务已恢复启动。', flush=True)
    print('计算文件校验值……', flush=True)
    for file in sorted(output.rglob('*')):
        if file.is_file():
            manifest['sha256'][file.relative_to(output).as_posix()] = digest(file)
    (output / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding='utf-8')
    print(f'完成：{output}\n这是私人完整备份，包含账号、个人资料和密钥，切勿提交公开GitHub。')


def restore_environment(args):
    bundle = Path(args.bundle).expanduser().resolve()
    project = Path(args.project_directory).expanduser().resolve()
    name = args.project_name
    if not re.fullmatch(r'exam408-copy-[a-z0-9-]+', name):
        raise ValueError('项目名必须以exam408-copy-开头，只能包含小写字母、数字和连字符')
    if any(os.environ.get(key) for key in ('COMPOSE_FILE', 'COMPOSE_PROJECT_NAME', 'APP_ENV_FILE', 'COMPOSE_ENV_FILES')):
        raise ValueError('当前终端设置了Compose环境覆盖，请在普通新终端恢复，避免连到其他项目')
    manifest = json.loads((bundle / 'manifest.json').read_text(encoding='utf-8'))
    validate_manifest(manifest)
    info = json_command(['docker', 'info', '--format', '{{json .}}'])
    if info['OSType'] != 'linux' or info['Architecture'] != manifest['architecture']:
        raise ValueError('两台Docker必须同为Linux容器且CPU架构相同，不能直接跨架构恢复物理数据库')
    if project.exists():
        raise ValueError('目标项目目录已存在，拒绝覆盖；请选择新目录')
    existing = set(command(['docker', 'volume', 'ls', '-q']).splitlines())
    volume_names = {key: name + '_' + key for key in VOLUMES}
    if existing.intersection(volume_names.values()):
        raise ValueError('目标数据卷已经存在，拒绝覆盖；请换一个新的--project-name和目录')
    if command(['docker', 'ps', '-a', '-q', '--filter', f'label=com.docker.compose.project={name}']):
        raise ValueError('目标Compose项目已经存在，拒绝覆盖')
    print('校验备份文件，请稍候……', flush=True)
    for path, expected in manifest['sha256'].items():
        file = bundle / path
        if not file.is_file() or file.is_symlink() or digest(file) != expected:
            raise ValueError('备份损坏或缺失：' + path)
    if not args.skip_ollama and manifest.get('ollama'):
        # Do not overwrite a colleague's different model manifests.
        models = model_directory()
        for source in (bundle / 'ollama-models').rglob('*'):
            if source.is_file():
                target = models / source.relative_to(bundle / 'ollama-models')
                if target.exists() and digest(target) != digest(source):
                    raise ValueError('本机存在不同的Ollama模型文件；请先核对，或用--skip-ollama保留现有模型')
        shutil.copytree(bundle / 'ollama-models', models, dirs_exist_ok=True)
    project.parent.mkdir(parents=True, exist_ok=True)
    command(['git', 'clone', bundle / 'code.bundle', project])
    command(['git', 'remote', 'set-url', 'origin', REMOTE], cwd=project)
    command(['git', 'checkout', '--detach', manifest['source_commit']], cwd=project)
    shutil.copy2(bundle / 'runtime.env', project / '.env')
    command(['docker', 'image', 'load', '-i', bundle / 'images.tar'])
    for key, volume in volume_names.items():
        command(['docker', 'volume', 'create', volume])
        print('恢复数据卷：' + key, flush=True)
        archive_volume(manifest['images']['redis'], volume, bundle, key, restore=True)
    override = {'services': {s: {'image': tag, 'pull_policy': 'never'} for s, tag in manifest['images'].items()},
                'volumes': {key: {'external': True, 'name': volume} for key, volume in volume_names.items()}}
    (project / 'compose.transfer.json').write_text(json.dumps(override, indent=2), encoding='utf-8')
    # Keep later plain `docker compose` commands on the imported images and volumes.
    with (project / '.env').open('a', encoding='utf-8') as stream:
        stream.write('\nAPP_ENV_FILE=.env\nCOMPOSE_PROJECT_NAME=' + name + '\nCOMPOSE_FILE=compose.yaml,compose.transfer.json\nCOMPOSE_PATH_SEPARATOR=,\n')
    print('启动导入的原版镜像，不重新构建、不执行文档重处理。', flush=True)
    command(['docker', 'compose', '-p', name, '-f', 'compose.yaml', '-f', 'compose.transfer.json', 'up', '-d', '--no-build', '--pull', 'never'], cwd=project)
    print(f'恢复完成：{project}\n打开 http://localhost:5173，使用原系统已有账号登录。\n问答还需要本机安装并启动Ollama；外部模型API需相应网络和配置。\n源码检出为备份版本；开发前用 git switch -c 创建自己的分支。')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='action', required=True)
    export = sub.add_parser('export')
    export.add_argument('--output', required=True)
    export.add_argument('--include-ollama', action='store_true')
    restore = sub.add_parser('restore')
    restore.add_argument('--bundle', required=True)
    restore.add_argument('--project-directory', required=True)
    restore.add_argument('--project-name', default='exam408-copy-local')
    restore.add_argument('--skip-ollama', action='store_true')
    args = parser.parse_args()
    try:
        if args.action == 'export': export_environment(args)
        else: restore_environment(args)
    except (ValueError, RuntimeError, OSError, KeyError) as error:
        raise SystemExit('未完成：' + str(error)) from None


if __name__ == '__main__':
    main()
