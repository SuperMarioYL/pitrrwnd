import pathlib, subprocess, tempfile
with tempfile.TemporaryDirectory(prefix="pitr-demo-") as folder:
    root = pathlib.Path(folder)
    binary = root / "pitr"
    subprocess.run(["go", "build", "-o", str(binary), "./cmd/pitr"], check=True)
    work = root / "work"; work.mkdir()
    def run(*args, expected=0):
        result = subprocess.run([str(binary), "--root", str(work), *args], text=True, capture_output=True)
        assert result.returncode == expected, result.stderr
        return result
    run("init")
    (work / "app.conf").write_text("port: 8080\n")
    (work / "state.txt").write_text("rows=42\n")
    run("savepoint", "--label", "before edits")
    (work / "app.conf").write_text("BROKEN\n")
    (work / "state.txt").unlink()
    (work / "junk.txt").write_text("unwanted\n")
    print("verify after edits:", run("verify", "--step", "1", expected=1).returncode)
    run("rewind", "--step", "1")
    print(run("verify", "--step", "1").stdout.strip())
    print("config restored:", (work / "app.conf").read_text() == "port: 8080\n")
    print("deleted file restored:", (work / "state.txt").read_text() == "rows=42\n")
    print("added file removed:", not (work / "junk.txt").exists())
