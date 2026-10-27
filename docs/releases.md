# Releases / Релизы

## English

Versions follow [SemVer](https://semver.org/): `MAJOR.MINOR.PATCH`. The single source of the version is `version` in `pyproject.toml`; `autobrowser version` and the HTTP API read it from package metadata. A release is a git tag `vX.Y.Z` on `main`.

### Making a release

1. Bump `version` in `pyproject.toml` and commit: `git commit -am "Release 0.2.0"`.
2. Create an annotated tag: `git tag -a v0.2.0 -m "v0.2.0"`.
3. Push the commit and the tag: `git push origin main v0.2.0`.

The `.github/workflows/release.yml` workflow runs on the tag. It checks that the tag matches the version in `pyproject.toml`, builds the wheel, the sdist and single-file binaries (`scripts/build-binary.sh` with PyInstaller on Linux amd64/arm64, macOS arm64 and Windows amd64, each with a `.sha256`), and creates a GitHub Release with these files. The release notes list the commit subjects since the previous tag.

If the tag does not match the version, the workflow fails and no release is created. Delete the tag (`git tag -d v0.2.0 && git push origin :refs/tags/v0.2.0`), fix the version and tag again.

### Installing a specific version

Add `@<tag>` to the git URL:

```bash
uvx --from "autobrowser-client[cloak] @ git+https://github.com/foxzi/autobrowser-client@v0.1.0" autobrowser mcp
```

Without a tag uv installs the latest commit of `main`.

## Русский

Версии по [SemVer](https://semver.org/lang/ru/): `MAJOR.MINOR.PATCH`. Единственный источник версии — поле `version` в `pyproject.toml`; `autobrowser version` и HTTP API берут её из метаданных пакета. Релиз — это git-тег `vX.Y.Z` на `main`.

### Как выпустить релиз

1. Поднять `version` в `pyproject.toml` и закоммитить: `git commit -am "Release 0.2.0"`.
2. Создать аннотированный тег: `git tag -a v0.2.0 -m "v0.2.0"`.
3. Отправить коммит и тег: `git push origin main v0.2.0`.

На тег запускается workflow `.github/workflows/release.yml`. Он проверяет, что тег совпадает с версией в `pyproject.toml`, собирает wheel, sdist и бинари одним файлом (`scripts/build-binary.sh` через PyInstaller на Linux amd64/arm64, macOS arm64 и Windows amd64, к каждому `.sha256`) и создаёт GitHub Release с этими файлами. В заметках к релизу — заголовки коммитов с предыдущего тега.

Если тег не совпадает с версией, workflow падает и релиз не создаётся. Удалите тег (`git tag -d v0.2.0 && git push origin :refs/tags/v0.2.0`), исправьте версию и поставьте тег заново.

### Установка конкретной версии

Добавьте `@<тег>` к git-ссылке:

```bash
uvx --from "autobrowser-client[cloak] @ git+https://github.com/foxzi/autobrowser-client@v0.1.0" autobrowser mcp
```

Без тега uv ставит последний коммит `main`.
