#!/usr/bin/env npx tsx
// Integration test: starts the Go API server, exercises every SDK method, validates responses.
// Usage: npx tsx test-integration.ts

import { spawn, ChildProcess } from "child_process";
import {
  TaskmanagerSdkClient,
  NotFoundError,
  UnprocessableEntityError,
  PermissionDeniedError,
} from "./generated/typescript/src/index";

// ── Helpers ──────────────────────────────────────────────────────────

let server: ChildProcess | null = null;
let passed = 0;
let failed = 0;

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(`Assertion failed: ${msg}`);
}

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
  }
}

async function test(name: string, fn: () => Promise<void>) {
  try {
    await fn();
    console.log(`  ✓ ${name}`);
    passed++;
  } catch (e: any) {
    console.log(`  ✗ ${name}: ${e.message}`);
    failed++;
  }
}

async function startServer(): Promise<void> {
  return new Promise((resolve, reject) => {
    server = spawn("go", ["run", "main.go"], {
      cwd: __dirname,
      stdio: ["ignore", "pipe", "pipe"],
    });
    const timeout = setTimeout(() => reject(new Error("Server start timeout")), 10000);

    server.stderr!.on("data", (data: Buffer) => {
      // Go server prints to stdout, but just in case
    });
    server.stdout!.on("data", (data: Buffer) => {
      if (data.toString().includes("listening on")) {
        clearTimeout(timeout);
        resolve();
      }
    });
    server.on("error", (err) => {
      clearTimeout(timeout);
      reject(err);
    });
  });
}

function stopServer() {
  if (server) {
    server.kill("SIGTERM");
    server = null;
  }
}

// ── Tests ────────────────────────────────────────────────────────────

async function run() {
  console.log("\n=== Starting Go API server ===\n");
  await startServer();
  console.log("Server ready on :8080\n");

  const client = new TaskmanagerSdkClient({
    baseUrl: "http://localhost:8080",
    apiKey: "test-api-key",
    maxRetries: 0,
  });

  // Client with no API key (for auth tests)
  const noAuthClient = new TaskmanagerSdkClient({
    baseUrl: "http://localhost:8080",
    apiKey: "",
    maxRetries: 0,
  });

  console.log("── CRUD Operations ──\n");

  let taskId: string;

  await test("listTasks returns empty list initially", async () => {
    const resp = await client.listTasks();
    assertEqual(resp.total, 0, "total");
    assertEqual(resp.tasks.length, 0, "tasks.length");
  });

  await test("createTask creates a task with all fields", async () => {
    const task = await client.createTask({
      title: "Buy groceries",
      description: "Milk, eggs, bread",
      priority: 3,
    });
    assert(typeof task.id === "string" && task.id.length > 0, "id is non-empty string");
    assertEqual(task.title, "Buy groceries", "title");
    assertEqual(task.description, "Milk, eggs, bread", "description");
    assertEqual(task.status, "pending", "default status");
    assertEqual(task.priority, 3, "priority");
    assert(typeof task.created_at === "string", "created_at is string");
    assert(typeof task.updated_at === "string", "updated_at is string");
    taskId = task.id;
  });

  await test("createTask creates a minimal task", async () => {
    const task = await client.createTask({ title: "Minimal task" });
    assertEqual(task.title, "Minimal task", "title");
    assertEqual(task.priority, 0, "default priority");
    assertEqual(task.status, "pending", "status");
  });

  await test("listTasks returns both tasks", async () => {
    const resp = await client.listTasks();
    assertEqual(resp.total, 2, "total");
    assertEqual(resp.tasks.length, 2, "tasks.length");
  });

  await test("getTask returns the created task", async () => {
    const task = await client.getTask(taskId);
    assertEqual(task.id, taskId, "id");
    assertEqual(task.title, "Buy groceries", "title");
    assertEqual(task.description, "Milk, eggs, bread", "description");
  });

  await test("updateTask changes title and status", async () => {
    const task = await client.updateTask(taskId, {
      title: "Buy organic groceries",
      status: "in_progress",
    });
    assertEqual(task.id, taskId, "id unchanged");
    assertEqual(task.title, "Buy organic groceries", "title updated");
    assertEqual(task.status, "in_progress", "status updated");
    assertEqual(task.description, "Milk, eggs, bread", "description unchanged");
  });

  await test("updateTask changes priority only", async () => {
    const task = await client.updateTask(taskId, { priority: 10 });
    assertEqual(task.priority, 10, "priority updated");
    assertEqual(task.title, "Buy organic groceries", "title unchanged");
  });

  await test("getTask reflects updates", async () => {
    const task = await client.getTask(taskId);
    assertEqual(task.title, "Buy organic groceries", "title");
    assertEqual(task.status, "in_progress", "status");
    assertEqual(task.priority, 10, "priority");
  });

  await test("deleteTask removes the task", async () => {
    await client.deleteTask(taskId);
    // Verify it's gone
    try {
      await client.getTask(taskId);
      throw new Error("Expected NotFoundError");
    } catch (e) {
      assert(e instanceof NotFoundError, `expected NotFoundError, got ${(e as Error).constructor.name}`);
    }
  });

  await test("listTasks has one task remaining", async () => {
    const resp = await client.listTasks();
    assertEqual(resp.total, 1, "total");
  });

  console.log("\n── Error Handling ──\n");

  await test("getTask 404 throws NotFoundError", async () => {
    try {
      await client.getTask("00000000-0000-0000-0000-000000000000");
      throw new Error("Expected NotFoundError");
    } catch (e) {
      assert(e instanceof NotFoundError, `expected NotFoundError, got ${(e as Error).constructor.name}`);
      assertEqual((e as NotFoundError).status, 404, "status");
      // Check errorMessage accessor works
      const msg = (e as any).errorMessage;
      assert(typeof msg === "string" && msg.length > 0, `errorMessage should be non-empty string, got: ${msg}`);
    }
  });

  await test("deleteTask 404 throws NotFoundError", async () => {
    try {
      await client.deleteTask("00000000-0000-0000-0000-000000000000");
      throw new Error("Expected NotFoundError");
    } catch (e) {
      assert(e instanceof NotFoundError, `expected NotFoundError, got ${(e as Error).constructor.name}`);
    }
  });

  await test("createTask without title throws UnprocessableEntityError", async () => {
    try {
      // @ts-expect-error — deliberately omitting required field
      await client.createTask({});
      throw new Error("Expected UnprocessableEntityError");
    } catch (e) {
      assert(
        e instanceof UnprocessableEntityError,
        `expected UnprocessableEntityError, got ${(e as Error).constructor.name}`
      );
      assertEqual((e as UnprocessableEntityError).status, 422, "status");
    }
  });

  await test("updateTask with invalid status throws UnprocessableEntityError", async () => {
    // Need a task to update
    const task = await client.createTask({ title: "temp" });
    try {
      await client.updateTask(task.id, { status: "invalid_status" });
      throw new Error("Expected UnprocessableEntityError");
    } catch (e) {
      assert(
        e instanceof UnprocessableEntityError,
        `expected UnprocessableEntityError, got ${(e as Error).constructor.name}`
      );
    }
    await client.deleteTask(task.id);
  });

  await test("request without auth throws PermissionDeniedError", async () => {
    try {
      await noAuthClient.listTasks();
      throw new Error("Expected PermissionDeniedError");
    } catch (e) {
      assert(
        e instanceof PermissionDeniedError,
        `expected PermissionDeniedError, got ${(e as Error).constructor.name}`
      );
      assertEqual((e as PermissionDeniedError).status, 403, "status");
    }
  });

  console.log("\n── Response Shape Validation ──\n");

  await test("Task has all expected fields with correct types", async () => {
    const task = await client.createTask({
      title: "Type check",
      description: "verify shapes",
      priority: 5,
    });

    // Required fields
    assert(typeof task.id === "string", "id is string");
    assert(typeof task.title === "string", "title is string");
    assert(typeof task.status === "string", "status is string");
    assert(typeof task.priority === "number", "priority is number");
    assert(typeof task.created_at === "string", "created_at is string");
    assert(typeof task.updated_at === "string", "updated_at is string");

    // Optional fields
    assert(typeof task.description === "string", "description is string");

    // UUID format
    assert(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(task.id), "id is UUID");

    // ISO 8601 timestamps
    assert(!isNaN(Date.parse(task.created_at)), "created_at is parseable date");
    assert(!isNaN(Date.parse(task.updated_at)), "updated_at is parseable date");

    await client.deleteTask(task.id);
  });

  await test("TaskListResponse has tasks array and total", async () => {
    const resp = await client.listTasks();
    assert(Array.isArray(resp.tasks), "tasks is array");
    assert(typeof resp.total === "number", "total is number");
    assertEqual(resp.total, resp.tasks.length, "total matches tasks.length");
  });

  // ── Summary ──

  console.log(`\n${"━".repeat(40)}`);
  console.log(`  ${passed} passed, ${failed} failed`);
  console.log(`${"━".repeat(40)}\n`);

  stopServer();
  process.exit(failed > 0 ? 1 : 0);
}

run().catch((err) => {
  console.error("Fatal:", err);
  stopServer();
  process.exit(1);
});
