#!/usr/bin/env node

const { run } = require("./index.cjs");

let child;
try {
  child = run(process.argv.slice(2));
} catch (err) {
  console.error(err && err.message ? err.message : String(err));
  process.exit(1);
}

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 1);
});

child.on("error", (err) => {
  console.error(err && err.message ? err.message : String(err));
  process.exit(1);
});
