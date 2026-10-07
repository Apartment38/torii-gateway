#!/usr/bin/env python3
"""Observe the installed Hermes CLI with an isolated profile and offline guards.

Uses a byte-for-byte source copy because the installed CLI also loads its
checkout's .env. Never reads that file or the user's profile. No account calls.
The host's selected dependency packages are read-only inputs, not a fresh install.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path('/tmp/hermes-observed-setup')
HOST = Path('/home/erick/.hermes/hermes-agent')
PIN = '562dc2a27aa22977e0795809a17fbbbb1819f0ab'


def env():
    values = {'PATH': '/usr/bin:/bin', 'LANG': 'C.UTF-8',
            'HERMES_HOME': str(ROOT / 'profile'),
            'OPENROUTER_API_KEY': 'offline-synthetic-key',
            'HERMES_MANAGED_DIR': str(ROOT / 'managed'),
            'HERMES_RUNTIME_DIR': str(ROOT / 'runtime-store'),
            'HERMES_DISABLE_LAZY_INSTALLS': '1', 'UV_OFFLINE': '1',
            'XDG_CONFIG_HOME': str(ROOT / 'xdg-config'),
            'XDG_CACHE_HOME': str(ROOT / 'xdg-cache'),
            'XDG_DATA_HOME': str(ROOT / 'xdg-data'), 'TMPDIR': str(ROOT),
            'GIT_CONFIG_GLOBAL': '/dev/null', 'GIT_CONFIG_SYSTEM': '/dev/null',
            'GIT_TERMINAL_PROMPT': '0', 'UV_CACHE_DIR': str(ROOT / 'uv-cache'),
            'UV_PYTHON_DOWNLOADS': 'never', 'UV_NO_CONFIG': '1'}
    if os.environ.get('REFERENCE_DEPENDENCY_DOWNLOADS') == '1':
        values['REFERENCE_DEPENDENCY_DOWNLOADS'] = '1'
        values.pop('UV_OFFLINE')
    return values


def guarded_child(source, argv):
    sys.dont_write_bytecode = True
    root = ROOT.resolve()
    events = []
    downloads = os.environ.get('REFERENCE_DEPENDENCY_DOWNLOADS') == '1'
    if downloads and Path(source).resolve() != (ROOT / 'host-source').resolve():
        raise RuntimeError('Dependency admission requires isolated source copy')

    def inside(path):
        return Path(os.fsdecode(path)).resolve().is_relative_to(root)

    def block(event, detail):
        events.append({'event': event, 'detail': str(detail)})
        raise PermissionError('offline isolation guard: ' + event + ': ' + str(detail))

    def audit(event, args):
        if event == 'open' and isinstance(args[0], (str, bytes, os.PathLike)):
            path, mode, flags = args
            if os.fsdecode(path) != '/dev/null' and not inside(path):
                name = Path(os.fsdecode(path)).name
                if name in {'.env', '.op.env', '.netrc', 'config.yaml', 'auth.json', 'credentials.json'}:
                    block(event, 'shared configuration/credential read refused')
                if flags & (os.O_WRONLY | os.O_RDWR | os.O_CREAT | os.O_TRUNC | os.O_APPEND):
                    block(event, path)
        if event in {'os.mkdir', 'os.remove', 'os.rmdir', 'os.chmod', 'os.truncate'}:
            if isinstance(args[0], (str, bytes)) and not inside(args[0]):
                block(event, args[0])
        if event in {'os.rename', 'os.link', 'os.symlink'}:
            if not inside(args[1]):
                block(event, args[1])
        if event == 'socket.getaddrinfo' and downloads:
            if str(args[0]).endswith(('github.com', 'githubusercontent.com', 'pypi.org', 'pythonhosted.org')):
                return
        if event == 'socket.connect' and downloads:
            return  # dependency URLs are resolved through the hostname gate above
        if event in {'socket.connect', 'socket.getaddrinfo', 'socket.bind'}:
            block(event, 'network disabled')
        if event == 'subprocess.Popen':
            executable, command, cwd, child_env = args
            if downloads and inside(executable) and (not cwd or inside(cwd)):
                return  # actual PM worker / uv; profile, source, store and caches all /tmp
            # The CLI's local Git clone and inspection are the only children
            # admitted. Dependency builders/secret helpers remain blocked.
            if Path(executable).name != 'git' or any(
                    str(a).startswith(('https:', 'http:', 'ssh:', 'git@')) for a in command):
                block(event, 'nonlocal/non-Git child refused: ' + str(command[0]))
            if cwd and not inside(cwd):
                block(event, 'Git cwd outside isolated tree')
    sys.addaudithook(audit)
    import atexit
    atexit.register(lambda: (ROOT / 'last-guard-events.json').write_text(json.dumps(events, indent=2)))
    sys.path.insert(0, source)
    if argv == ['__registry_fixture__']:
        registry_fixture()
        return
    sys.argv = ['hermes', *argv]
    import runpy
    runpy.run_module('hermes_cli.main', run_name='__main__', alter_sys=True)


def registry_fixture():
    import copy
    import importlib.util
    import dataclasses
    spec = importlib.util.spec_from_file_location('reference_fixture', Path(__file__).with_name('test-hermes-routing-reference.py'))
    fixture = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(fixture)
    from hermes_cli.plugins import get_plugin_manager
    from hermes_cli.middleware import apply_llm_request_middleware
    manager = get_plugin_manager()
    manager.discover_and_load()
    loaded = {name: {'error': plugin.error, 'deferred': plugin.deferred}
              for name, plugin in manager._plugins.items() if 'jev-effort-router' in name}
    callbacks = manager._middleware.get('llm_request', [])
    if len(callbacks) != 1:
        raise RuntimeError('Expected exactly one real registered llm_request middleware: ' + str(loaded))
    router = callbacks[0].__self__
    default_path = ROOT / 'actual-registry-default-settings.json'
    if not default_path.exists():
        default_path.write_text(json.dumps(dataclasses.asdict(router._get_settings()), indent=2) + '\n')
    default_settings = json.loads(default_path.read_text())
    # Observe supported host customization persistence/reload, using precisely
    # the same three eligible models as the frozen namespace harness.
    from hermes_cli.plugins_state import save_plugin_setting
    grid = [f"{m['id']}: quality {m['quality']}; tools {m['tools']}; reasoning {m['reasoning']}; cost {m['input_per_million']} per million" for m in fixture.POOL]
    save_plugin_setting('jev-effort-router', ('grid',), grid)
    save_plugin_setting('jev-effort-router', ('default_model',), 'cheap')
    (ROOT / 'profile/ollama_cloud_models_cache.json').write_text(json.dumps({'models': [{'id': m['id']} for m in fixture.POOL]}))
    manager.discover_and_load(force=True)
    callbacks = manager._middleware['llm_request']
    router = callbacks[0].__self__
    assert [entry.model_id for entry in router._get_settings().grid] == ['cheap', 'strong', 'backup']
    assert router._get_settings().default_model == 'cheap'
    client_module = sys.modules[type(router).__module__.rsplit('.', 1)[0] + '.client']
    transport = fixture.TypedTransport()
    router._client_factory = lambda settings: client_module.JevClient(settings, transport=transport)
    rows = []
    reference_path = Path(__file__).resolve().parent.parent / 'docs/gauntlet/hermes-reference-results.json'
    reference_bytes = reference_path.read_bytes()
    reference = json.loads(reference_bytes)
    matched = [row for row in reference['cases'] if row['scope'] == 'matched_routing']
    assert len(matched) == 6
    cases = matched + [row for row in reference['cases'] if row['case'] == 'contextual_followup']
    for frozen in cases:
        name, expected = frozen['case'], frozen['expected_model']
        request = copy.deepcopy(frozen['input'])
        request['model'] = 'cheap' if request['model'] == 'smart' else request['model'].removeprefix('gauntlet/')
        serialize = lambda body: json.dumps(body, separators=(',', ':'), ensure_ascii=False).encode()
        assert serialize(request) == serialize(frozen['host_input']), name + ': frozen host input differs'
        request_hash = hashlib.sha256(serialize(request)).hexdigest()
        turn = 'registered-' + name
        session = 'registered-affinity' if name.startswith('affinity_') else 'registered-' + name
        before = len(transport.records)
        result = apply_llm_request_middleware(request, model='cheap', provider='ollama-cloud',
                                              api_mode='chat_completions', turn_id=turn,
                                              session_id=session, api_call_count=1)
        rows.append({'case': name, 'host_input': request, 'request_output': result.payload,
                     'scope': frozen['scope'], 'mapped_host_input_byte_identical_to_frozen_reference': True,
                     'mapped_host_input_sha256': request_hash,
                     'turn_id': turn, 'session_id': session,
                     'expected_model': expected, 'matches_expected_model': result.payload['model'] == expected,
                     'changed': result.changed, 'trace': result.trace,
                     'decision_records': transport.records[before:]})
    assert reference_path.read_bytes() == reference_bytes
    output = {'registration': loaded, 'middleware_count': len(callbacks),
              'immutable_fixture_source': 'docs/gauntlet/hermes-reference-results.json',
              'immutable_fixture_source_sha256': hashlib.sha256(reference_bytes).hexdigest(),
              'byte_identical_matched_host_inputs': len(matched),
              'registered_tools': sorted(x for x in manager._plugin_tool_names if x.startswith('jev_effort_router_')),
              'registered_hooks': sorted(manager._hooks),
              'settings': dataclasses.asdict(router._get_settings()),
              'catalog_models': router._catalog.models(), 'cases': rows,
              'default_settings_before_customization': default_settings,
              'customization': 'actual host save_plugin_setting grid/default_model, then force reload and assert persisted values',
              'installation_method': 'actual CLI --force pinned installation and separate CLI enable in isolated source copy; initial manual staging was replaced by successful CLI publication',
              'real_host_registry_and_middleware_dispatch': True,
              'production_calls': 0}
    (ROOT / 'actual-registry-results.json').write_text(json.dumps(output, indent=2) + '\n')
    print(json.dumps({'registration': loaded, 'middleware_count': len(callbacks),
                      'cases': [{'case': row['case'], 'model': row['request_output']['model'],
                                 'decision_calls': len(row['decision_records'])} for row in rows]}))


def main():
    if len(sys.argv) > 1 and sys.argv[1] == '--guarded-child':
        guarded_child(sys.argv[2], sys.argv[3:])
        return
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--command', nargs=argparse.REMAINDER)
    p.add_argument('--use-host-source', action='store_true')
    a = p.parse_args()
    ROOT.mkdir(exist_ok=True)
    profile = ROOT / 'profile'
    profile.mkdir(exist_ok=True)
    if not (profile / '.env').exists():
        (profile / '.env').write_text('OPENROUTER_API_KEY=offline-synthetic-key\n')
    if not (profile / 'config.yaml').exists():
        (profile / 'config.yaml').write_text('plugins:\n  enabled: []\n')
    facts = json.loads((Path('/home/erick/.hermes/installs/618af23db65255f1/facts.json')).read_text())
    dependency_env = Path(facts['packages']['venv']['environment'])
    store_python = Path('/home/erick/.hermes/tools/python-3.14.7+20260901-linux-x64/bin/python3')
    runtime = ROOT / 'runtime'
    if not runtime.exists():
        subprocess.run([str(store_python), '-I', '-m', 'venv', '--without-pip', str(runtime)], check=True, env=env())
        site = runtime / 'lib/python3.14/site-packages'
        (site / 'host-readonly-dependencies.pth').write_text(str(dependency_env / 'lib/python3.14/site-packages') + '\n')
    source = ROOT / 'host-source'
    if not source.exists():
        files = subprocess.check_output(['git', '-C', str(HOST), 'ls-files', '-z'], env=env()).split(b'\0')
        source.mkdir()
        for raw in files:
            if not raw:
                continue
            relative = Path(os.fsdecode(raw))
            if relative.name in {'.env', '.op.env', 'auth.json', 'credentials.json'}:
                continue
            original = HOST / relative
            if original.is_file() and not original.is_symlink():
                target = source / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(original, target)
    command = a.command or ['plugins', '--help']
    result = subprocess.run([str(runtime / 'bin/python'), '-B', '-I', str(Path(__file__).resolve()),
                             '--guarded-child', str(HOST if a.use_host_source else source), *command],
                            cwd=ROOT, env=env(), capture_output=True, text=True, timeout=60)
    report = {'command': ['hermes', *command], 'source_mode': 'installed' if a.use_host_source else 'isolated byte-for-byte source copy',
              'host_commit': subprocess.check_output(['git', '-C', str(HOST), 'rev-parse', 'HEAD'], env=env(), text=True).strip(),
              'host_selected_dependency_environment': str(dependency_env),
              'host_python': str(store_python), 'profile': str(profile),
              'exit_code': result.returncode, 'stdout': result.stdout, 'stderr': result.stderr,
              'guard_events': json.loads((ROOT / 'last-guard-events.json').read_text()) if (ROOT / 'last-guard-events.json').exists() else [],
              'fresh_host_install': False, 'inference_or_account_calls': False,
              'dependency_downloads_allowed': os.environ.get('REFERENCE_DEPENDENCY_DOWNLOADS') == '1'}
    filename = ('host-' if a.use_host_source else 'isolated-') + '-'.join(x.replace('/', '_') for x in command[:2]) + '-' + hashlib.sha256(json.dumps(command).encode()).hexdigest()[:10] + '.json'
    (ROOT / filename).write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()
