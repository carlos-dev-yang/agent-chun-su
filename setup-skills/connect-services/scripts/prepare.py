#!/usr/bin/env python3
"""Prepare a local setup checklist; never install, authenticate, or call providers."""

import argparse
import json
import os
from pathlib import Path
import platform
import shutil
import sys


SKILL_ROOT = Path(__file__).resolve().parent.parent
CATALOG_PATH = SKILL_ROOT / "catalog.json"
MODES = {"assisted": "AI 안내 방식", "scripted": "고정 helper 방식 준비"}
INTENTS = {
    "read": "자료 읽기·요약",
    "write": "작성·수정·전송 기능도 필요 (실행 승인 아님)",
}
PRIVATE_FILE_MODE = 0o600
CANCEL_CHOICES = {"q", "quit", "cancel", "취소"}


def load_catalog():
    catalog = json.loads(CATALOG_PATH.read_text(encoding="utf-8"))
    services = catalog["services"]
    if not services or len({item["id"] for item in services}) != len(services):
        raise ValueError("서비스 목록이 없거나 ID가 중복됩니다.")
    for item in services:
        manual = (SKILL_ROOT / item["manual"]).resolve()
        if SKILL_ROOT not in manual.parents or not manual.is_file():
            raise ValueError("서비스 매뉴얼이 없거나 팩 밖을 가리킵니다.")
    return catalog


def select_services(value, services):
    names = [part.strip().lower() for part in value.split(",") if part.strip()]
    if names == ["all"]:
        return services
    available = {item["id"] for item in services}
    if not names or any(name not in available for name in names):
        raise ValueError("목록의 서비스 ID를 쉼표로 구분하거나 all을 입력하세요.")
    return [item for item in services if item["id"] in names]


def ask(prompt):
    print(prompt, file=sys.stderr, flush=True)
    response = input().strip()
    if response.lower() in CANCEL_CHOICES:
        raise KeyboardInterrupt
    return response


def choose(prompt, options, default):
    choices = ", ".join(f"{key}: {label}" for key, label in options.items())
    while True:
        response = ask(f"{prompt}\n{choices}\n기본값 {default}; q 취소")
        response = response or default
        if response in options:
            return response
        print("표시된 항목 중 하나를 입력하세요.", file=sys.stderr)


def render(catalog, selected, mode, intent):
    lines = [
        "# Chun-su 서비스 연결 준비표",
        "",
        f"매뉴얼 조사일: {catalog['reviewed_on']}; 현재 환경: {platform.system()} / {platform.machine()}",
        f"진행 방식: {MODES[mode]}; 사용 목적: {INTENTS[intent]}",
        "",
        "이 문서는 설치 계획이며 계정 연결·권한 승인·룰셋 검증 완료 기록이 아닙니다.",
        "선택한 프로그램의 PATH상 존재만 확인했습니다. 버전·인증·실행 가능성은 미확인입니다.",
        "토큰·비밀번호·OAuth code/callback URL을 이 문서에 적지 마세요.",
        "",
    ]
    if {item["id"] for item in selected} >= {"drive", "gmail"}:
        lines += [
            "Google 공통 질문: Drive와 Gmail에 같은 계정을 사용할지 먼저 확인하세요.",
            "기존 Gmail 읽기 권한을 다른 서비스 권한으로 자동 확대하거나 토큰을 복사하지 않습니다.",
            "",
        ]
    for item in selected:
        manual = (SKILL_ROOT / item["manual"]).resolve().as_posix()
        lines += [
            f"## {item['name']}",
            "",
            f"- 현재 제품 지원: {item['chunsu_status']}",
            "- 기존 계정 연결: 미확인 — 새 로그인 전에 재사용 여부를 확인하세요.",
            f"- [질문·설치·검증·해제 매뉴얼](<{manual}>)",
        ]
        for program in item["optional_programs"]:
            found = "PATH에서 발견" if shutil.which(program) else "PATH에서 미발견"
            lines.append(f"- 선택 경로의 도구 {program}: {found} (필수 설치 지시 아님)")
        lines += [
            "- 다음 입력: 매뉴얼의 ‘사용자에게 물을 내용’에서 계정·리소스 범위를 선택하세요.",
            "- 연결 검증: identity → 허용 리소스 호출 → 범위 밖 거부 → 기능 결과 확인.",
            "- 룰셋 검증: 실행기·지침·도구 버전과 독립 평가 근거가 있어야 별도 표시합니다.",
            "",
        ]
    lines += [
        "## 실제 연결로 이어가기",
        "",
        "선택 서비스의 매뉴얼을 설정 도우미에게 읽히거나 해당 공식 설치 절차를 진행하세요.",
        "고정 설치 helper와 Chun-su adapter가 미구현이면 외부 도구 설치와 제품 연결을 구별하세요.",
        "현재 일반 보고 실행기의 shell·추가 MCP 제한을 해제해서 설치하지 않습니다.",
        "전송/쓰기 검증은 대상과 내용에 대한 기존 승인을 확인하고 필요한 경우에만 요청합니다.",
        "",
    ]
    return "\n".join(lines)


def write_new(path, content):
    # Exclusive creation also refuses a pre-existing symlink. Never overwrite user work.
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, PRIVATE_FILE_MODE)
    with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
        handle.write(content)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--services", help="쉼표로 구분한 서비스 ID 또는 all")
    parser.add_argument("--mode", choices=MODES, default="assisted")
    parser.add_argument("--intent", choices=INTENTS, default="read")
    parser.add_argument("--interactive", action="store_true", help="서비스·목적·진행 방식 질문")
    parser.add_argument("--list", action="store_true", help="준비된 서비스와 지원 상태 표시")
    parser.add_argument("--output", type=Path, help="새 준비표 파일; 기존 파일은 덮어쓰지 않음")
    args = parser.parse_args()
    try:
        catalog = load_catalog()
        services = catalog["services"]
        if args.list:
            for item in services:
                print(f"{item['id']}: {item['name']} — {item['chunsu_status']}")
            return 0
        if args.interactive:
            print("연결 준비만 진행합니다. 로그인·토큰은 입력하지 마세요.", file=sys.stderr)
            for item in services:
                print(f"  {item['id']}: {item['name']}", file=sys.stderr)
            if args.services is None:
                while True:
                    value = ask("서비스 ID를 쉼표로 입력하세요 (all 전체, 빈 입력 건너뛰기, q 취소).")
                    if not value:
                        print("서비스 연결 준비를 건너뛰었습니다.", file=sys.stderr)
                        return 0
                    try:
                        select_services(value, services)
                        args.services = value
                        break
                    except ValueError as error:
                        print(str(error), file=sys.stderr)
            args.mode = choose("어떤 방식으로 진행할까요?", MODES, args.mode)
            args.intent = choose("어떤 기능이 필요한가요?", INTENTS, args.intent)
        if args.services is None:
            parser.error("--services 또는 --interactive를 선택하세요. --list로 목록을 볼 수 있습니다.")
        selected = select_services(args.services, services)
        content = render(catalog, selected, args.mode, args.intent)
        if args.output:
            write_new(args.output, content)
            print(f"준비표 저장: {args.output.resolve()}")
        else:
            print(content, end="")
        return 0
    except (EOFError, KeyboardInterrupt):
        print("준비를 취소했습니다. 연결이나 설정 변경은 수행하지 않았습니다.", file=sys.stderr)
        return 130
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f"준비표를 만들지 못했습니다: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
