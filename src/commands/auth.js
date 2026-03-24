/**
 * Auth commands: login, logout, status
 */

import fs from 'fs';
import http from 'http';
import path from 'path';
import { loadConfig, saveConfig, getValidToken, resolveCredentialsPath } from '../lib/auth.js';
import { jsonOutput, jsonError } from '../lib/output.js';

export function registerAuthCommands(program) {
  const auth = program.command('auth').description('Manage authentication');

  auth
    .command('status')
    .description('Check authentication status and token validity')
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const info = {
          user: config.displayName || null,
          phone: config.phoneNumber || null,
          userId: config.userId || null,
          configPath: resolveCredentialsPath(),
          tokenValid: false,
        };

        try {
          await getValidToken(config);
          info.tokenValid = true;
        } catch (e) {
          info.tokenError = e.message;
        }

        jsonOutput(info);
      } catch (e) {
        jsonError(e.message, 2, 'auth_error');
      }
    });

  auth
    .command('login')
    .description('Authenticate via bookmarklet (starts local server on port 9876)')
    .action(async () => {
      const configPath = resolveCredentialsPath();
      const configDir = path.dirname(configPath);
      if (!fs.existsSync(configDir)) {
        fs.mkdirSync(configDir, { recursive: true });
      }

      const PORT = 9876;

      const extractorCode = `(async function(){try{const dbReq=indexedDB.open('firebaseLocalStorageDb');dbReq.onsuccess=function(e){const db=e.target.result;const tx=db.transaction('firebaseLocalStorage','readonly');const store=tx.objectStore('firebaseLocalStorage');const getReq=store.getAll();getReq.onsuccess=function(){const items=getReq.result;const authItem=items.find(i=>i.fbase_key&&i.fbase_key.includes('firebase:authUser'));if(!authItem||!authItem.value){alert('No auth found. Make sure you are logged into Partiful.');return;}const v=authItem.value;const data={apiKey:v.apiKey,refreshToken:v.stsTokenManager?.refreshToken,userId:v.uid,displayName:v.displayName,phoneNumber:v.phoneNumber};if(!data.refreshToken){alert('No refresh token found. Try logging out and back in.');return;}fetch('http://localhost:${PORT}/auth',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)}).then(r=>r.ok?alert('Auth saved! You can close this tab.'):alert('Failed to save auth')).catch(()=>alert('Could not connect to CLI. Is it running?'));};};dbReq.onerror=()=>alert('Could not open IndexedDB');}catch(e){alert('Error: '+e.message);}})();`;

      const bookmarklet = 'javascript:' + encodeURIComponent(extractorCode);

      console.error(`
Partiful CLI Auth Setup
=======================

1. Open https://partiful.com and log in
2. Create a bookmarklet with this URL:

${bookmarklet}

3. Click the bookmarklet while on partiful.com

Waiting for auth data on http://localhost:${PORT}... (Ctrl+C to cancel)
`);

      return new Promise((resolve, reject) => {
        const server = http.createServer((req, res) => {
          res.setHeader('Access-Control-Allow-Origin', '*');
          res.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS');
          res.setHeader('Access-Control-Allow-Headers', 'Content-Type');

          if (req.method === 'OPTIONS') {
            res.writeHead(204);
            res.end();
            return;
          }

          if (req.method === 'POST' && req.url === '/auth') {
            let body = '';
            req.on('data', chunk => body += chunk);
            req.on('end', () => {
              try {
                const data = JSON.parse(body);
                if (!data.refreshToken || !data.userId) {
                  res.writeHead(400);
                  res.end('Missing required fields');
                  return;
                }

                const config = {
                  apiKey: data.apiKey || 'AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k',
                  refreshToken: data.refreshToken,
                  userId: data.userId,
                  displayName: data.displayName || 'Unknown',
                  phoneNumber: data.phoneNumber || 'Unknown',
                };

                saveConfig(config);
                res.writeHead(200);
                res.end('OK');

                jsonOutput({
                  user: config.displayName,
                  phone: config.phoneNumber,
                  configPath,
                });

                server.close();
                resolve();
              } catch (e) {
                res.writeHead(400);
                res.end('Invalid JSON');
              }
            });
          } else {
            res.writeHead(404);
            res.end('Not found');
          }
        });

        server.on('error', (e) => {
          if (e.code === 'EADDRINUSE') {
            jsonError(`Port ${PORT} is already in use`, 5, 'internal_error');
          } else {
            jsonError(e.message, 5, 'internal_error');
          }
          reject(e);
        });

        server.listen(PORT);
      });
    });

  auth
    .command('logout')
    .description('Remove stored credentials')
    .action(async () => {
      const configPath = resolveCredentialsPath();
      if (fs.existsSync(configPath)) {
        fs.unlinkSync(configPath);
        jsonOutput({ removed: true, configPath });
      } else {
        jsonOutput({ removed: false, message: 'Already logged out (no config found)' });
      }
    });
}
