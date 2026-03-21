package setup

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	_ "github.com/mattn/go-sqlite3"
)

// SyncGroups handles fetching group metadata and writing it to the DB.
// Ports logic from setup/groups.ts.
func SyncGroups(args []string) {
	projectRoot, _ := os.Getwd()

	var list bool
	limit := 30
	for i := 0; i < len(args); i++ {
		if args[i] == "--list" {
			list = true
		}
		if args[i] == "--limit" && i+1 < len(args) {
			if l, err := strconv.Atoi(args[i+1]); err == nil {
				limit = l
			}
			i++
		}
	}

	if list {
		listGroups(limit)
		return
	}

	syncGroupsInternal(projectRoot)
}

func listGroups(limit int) {
	dbPath := filepath.Join(config.StoreDir, "messages.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "ERROR: database not found\n")
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT jid, name FROM chats
		WHERE jid LIKE '%@g.us' AND jid <> '__group_sync__' AND name <> jid
		ORDER BY last_message_time DESC
		LIMIT ?`, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: query failed: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	for rows.Next() {
		var jid, name string
		if err := rows.Scan(&jid, &name); err != nil {
			continue
		}
		fmt.Printf("%s|%s\n", jid, name)
	}
}

func syncGroupsInternal(projectRoot string) {
	authDir := filepath.Join(projectRoot, "store", "auth")
	hasWhatsAppAuth := false
	if info, err := os.Stat(authDir); err == nil && info.IsDir() {
		files, err := os.ReadDir(authDir)
		if err == nil && len(files) > 0 {
			hasWhatsAppAuth = true
		}
	}

	if !hasWhatsAppAuth {
		logger.Info("WhatsApp auth not found — skipping group sync")
		EmitStatus("SYNC_GROUPS", map[string]interface{}{
			"BUILD":         "skipped",
			"SYNC":          "skipped",
			"GROUPS_IN_DB":  0,
			"REASON":        "whatsapp_not_configured",
			"STATUS":        "success",
			"LOG":           "logs/setup.log",
		})
		return
	}

	// Build TypeScript first
	logger.Info("Building TypeScript")
	buildOk := false
	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = projectRoot
	if err := cmd.Run(); err == nil {
		buildOk = true
		logger.Info("Build succeeded")
	} else {
		logger.Error("Build failed", map[string]interface{}{"err": err})
		EmitStatus("SYNC_GROUPS", map[string]interface{}{
			"BUILD":        "failed",
			"SYNC":         "skipped",
			"GROUPS_IN_DB": 0,
			"STATUS":       "failed",
			"ERROR":        "build_failed",
			"LOG":          "logs/setup.log",
		})
		os.Exit(1)
	}

	// Run sync script via a temp file
	logger.Info("Fetching group metadata")
	syncOk := false
	
	syncScript := `
import makeWASocket, { useMultiFileAuthState, makeCacheableSignalKeyStore, Browsers } from '@whiskeysockets/baileys';
import pino from 'pino';
import path from 'path';
import fs from 'fs';
import Database from 'better-sqlite3';

const logger = pino({ level: 'silent' });
const authDir = path.join('store', 'auth');
const dbPath = path.join('store', 'messages.db');

if (!fs.existsSync(authDir)) {
  console.error('NO_AUTH');
  process.exit(1);
}

const db = new Database(dbPath);
db.pragma('journal_mode = WAL');
db.exec('CREATE TABLE IF NOT EXISTS chats (jid TEXT PRIMARY KEY, name TEXT, last_message_time TEXT)');

const upsert = db.prepare(
  'INSERT INTO chats (jid, name, last_message_time) VALUES (?, ?, ?) ON CONFLICT(jid) DO UPDATE SET name = excluded.name'
);

const { state, saveCreds } = await useMultiFileAuthState(authDir);

const sock = makeWASocket({
  auth: { creds: state.creds, keys: makeCacheableSignalKeyStore(state.keys, logger) },
  printQRInTerminal: false,
  logger,
  browser: Browsers.macOS('Chrome'),
});

const timeout = setTimeout(() => {
  console.error('TIMEOUT');
  process.exit(1);
}, 30000);

sock.ev.on('creds.update', saveCreds);

sock.ev.on('connection.update', async (update) => {
  if (update.connection === 'open') {
    try {
      const groups = await sock.groupFetchAllParticipating();
      const now = new Date().toISOString();
      let count = 0;
      for (const [jid, metadata] of Object.entries(groups)) {
        if (metadata.subject) {
          upsert.run(jid, metadata.subject, now);
          count++;
        }
      }
      console.log('SYNCED:' + count);
    } catch (err) {
      console.error('FETCH_ERROR:' + err.message);
    } finally {
      clearTimeout(timeout);
      sock.end(undefined);
      db.close();
      process.exit(0);
    }
  } else if (update.connection === 'close') {
    clearTimeout(timeout);
    console.error('CONNECTION_CLOSED');
    process.exit(1);
  }
});
`
	tmpScript := filepath.Join(projectRoot, ".tmp-group-sync.mjs")
	if err := os.WriteFile(tmpScript, []byte(syncScript), 0644); err == nil {
		defer os.Remove(tmpScript)
		
		cmd := exec.Command("node", tmpScript)
		cmd.Dir = projectRoot
		out, err := cmd.CombinedOutput()
		if err == nil && strings.Contains(string(out), "SYNCED:") {
			syncOk = true
			logger.Info("Sync output", map[string]interface{}{"output": strings.TrimSpace(string(out))})
		} else {
			logger.Error("Sync failed", map[string]interface{}{"err": err, "output": string(out)})
		}
	} else {
		logger.Error("Failed to write temp sync script", map[string]interface{}{"err": err})
	}

	// Count groups in DB
	groupsInDb := 0
	dbPath := filepath.Join(config.StoreDir, "messages.db")
	if _, err := os.Stat(dbPath); err == nil {
		db, err := sql.Open("sqlite3", dbPath)
		if err == nil {
			defer db.Close()
			err = db.QueryRow("SELECT COUNT(*) FROM chats WHERE jid LIKE '%@g.us' AND jid <> '__group_sync__'").Scan(&groupsInDb)
			if err != nil {
				// Ignore error if table doesn't exist
			}
		}
	}

	status := "failed"
	if syncOk {
		status = "success"
	}

	EmitStatus("SYNC_GROUPS", map[string]interface{}{
		"BUILD":        func() string { if buildOk { return "success" }; return "failed" }(),
		"SYNC":         func() string { if syncOk { return "success" }; return "failed" }(),
		"GROUPS_IN_DB": groupsInDb,
		"STATUS":       status,
		"LOG":          "logs/setup.log",
	})

	if status == "failed" {
		os.Exit(1)
	}
}
