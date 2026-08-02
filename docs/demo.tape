Output assets/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 120
Set Height 30
Set Padding 14

# Requires /tmp/pitr to be pre-built (see Makefile `demo` target or demo.yml).

Type "rm -rf /tmp/pitrdemo && mkdir -p /tmp/pitrdemo && cd /tmp/pitrdemo"
Enter
Sleep 1s

Type "/tmp/pitr init"
Enter
Sleep 1s

Type "echo 'port: 8080' > app.conf && echo v1 > ver.txt && mkdir -p db && echo 'rows=42' > db/store.db"
Enter
Sleep 1s

Type "/tmp/pitr savepoint --label 'before risky refactor'"
Enter
Sleep 1s

Type "echo 'port: 9999' > app.conf && echo BROKEN >> app.conf && echo 'agent junk' > junk.txt && rm db/store.db"
Enter
Sleep 1s

Type "/tmp/pitr verify --step 1 || true"
Enter
Sleep 2s

Type "/tmp/pitr rewind --step 1"
Enter
Sleep 1s

Type "/tmp/pitr verify --step 1"
Enter
Sleep 2s

Type "/tmp/pitr log"
Enter
Sleep 3s
