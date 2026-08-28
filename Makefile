.PHONY: audit inspect-config install-icon read-imagegen-skill format test build verify bot-info configure plan download repo-plan repo-push

audit:
	@python3 scripts/codex_repo_admin.py audit

inspect-config:
	@python3 scripts/codex_repo_admin.py inspect-config

install-icon:
	@python3 scripts/codex_repo_admin.py install-icon

test:
	@python3 scripts/codex_repo_admin.py test

format:
	@python3 scripts/codex_repo_admin.py format

build:
	@python3 scripts/codex_repo_admin.py build

verify:
	@python3 scripts/codex_repo_admin.py verify

bot-info:
	@python3 scripts/codex_repo_admin.py run-bot info

configure:
	@python3 scripts/codex_repo_admin.py run-bot configure

plan:
	@python3 scripts/codex_repo_admin.py run-bot plan

download:
	@python3 scripts/codex_repo_admin.py run-bot download

repo-plan:
	@python3 scripts/codex_repo_admin.py repo-plan

repo-push:
	@python3 scripts/codex_repo_admin.py repo-push

read-imagegen-skill:
	@python3 scripts/codex_repo_admin.py read-imagegen-skill
