# Publishing Releases

## Binarios a compilar

Cada release debe incluir binarios para ambos comandos:

- `syspeek` - Servidor principal
- `syspeek_hash` - Utilidad para generar hashes de passwords

## Plataformas soportadas

| OS | Arquitectura | Nombre del binario |
|----|--------------|-------------------|
| Linux | amd64 | `syspeek-linux-amd64` |
| Linux | arm64 | `syspeek-linux-arm64` |
| macOS | amd64 (Intel) | `syspeek-darwin-amd64` |
| macOS | arm64 (Apple Silicon) | `syspeek-darwin-arm64` |
| Windows | amd64 | `syspeek-windows-amd64.exe` |

Lo mismo para `syspeek_hash-*`.

## Proceso de release

1. Actualizar versión en `main.go`:
   ```go
   const Version = "X.Y.Z"
   ```

2. Commit y push:
   ```bash
   git add -A && git commit -m "Release vX.Y.Z" && git push
   ```

3. Compilar binarios:
   ```bash
   mkdir -p dist && rm -f dist/*

   # syspeek
   GOOS=linux GOARCH=amd64 go build -o dist/syspeek-linux-amd64 . &
   GOOS=linux GOARCH=arm64 go build -o dist/syspeek-linux-arm64 . &
   GOOS=darwin GOARCH=amd64 go build -o dist/syspeek-darwin-amd64 . &
   GOOS=darwin GOARCH=arm64 go build -o dist/syspeek-darwin-arm64 . &
   GOOS=windows GOARCH=amd64 go build -o dist/syspeek-windows-amd64.exe . &

   # syspeek_hash
   GOOS=linux GOARCH=amd64 go build -o dist/syspeek_hash-linux-amd64 ./cmd/syspeek_hash/ &
   GOOS=linux GOARCH=arm64 go build -o dist/syspeek_hash-linux-arm64 ./cmd/syspeek_hash/ &
   GOOS=darwin GOARCH=amd64 go build -o dist/syspeek_hash-darwin-amd64 ./cmd/syspeek_hash/ &
   GOOS=darwin GOARCH=arm64 go build -o dist/syspeek_hash-darwin-arm64 ./cmd/syspeek_hash/ &
   GOOS=windows GOARCH=amd64 go build -o dist/syspeek_hash-windows-amd64.exe ./cmd/syspeek_hash/ &

   wait
   ```

4. Crear tag:
   ```bash
   git tag -a vX.Y.Z -m "Release vX.Y.Z"
   git push origin vX.Y.Z
   ```

5. Crear release en GitHub:
   ```bash
   gh release create vX.Y.Z dist/* --title "Syspeek vX.Y.Z" --notes "Release notes..."
   ```

## Checklist pre-release

- [ ] Versión actualizada en `main.go`
- [ ] `./syspeek -v` muestra la versión correcta
- [ ] Tests pasan (si los hay)
- [ ] Changelog/release notes preparados
- [ ] Ambos binarios compilados para las 5 plataformas (10 archivos total)
