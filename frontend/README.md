# Lab frontend

React, TypeScript, Vite, Tailwind CSS, and Monaco Editor, connected to the existing Go backtesting API.

Requires Node.js 22.12+ (or a newer supported LTS release).

```sh
cd frontend
npm install
npm run dev
```

Open the local URL printed by Vite (normally http://localhost:5173).
On Windows PowerShell, use `npm.cmd` if execution policy blocks `npm`.

```sh
npm run build
npm run preview
```

## Backend connection

Run the Go server separately on port 8080. Vite proxies `/api` to it during development, so no backend CORS changes are required.

To use another backend address, copy `.env.example` to `.env.local` and change:

```text
VITE_API_PROXY_TARGET=http://localhost:8080
```

You can also set this variable in the shell. Restart Vite after changing it.
All API calls send `X-User-ID: 1`, centralized in `src/config.ts`. User 1 must already exist in PostgreSQL; this temporary development identifier is not authentication.

The actual backend routes are unversioned (`/api/backtests`), and IDs are strings. Supported statuses are `PENDING`, `COMPLETED`, and `FAILED`.

## User flows

- **Editor**: Run Backtest submits the current script and opens `/backtests/:id` immediately after acceptance. Submission failures leave the script intact. Draft edits survive navigation but reset on page reload.
- **Backtest detail**: Fetches immediately and checks again every three seconds while pending, without overlapping requests. Polling stops on completion, failure, or navigation. Temporary errors during pending polling retry at the same interval; initial load errors offer a Retry button. Navigating away cancels frontend requests, not backend execution.
- **My Backtests**: Loads the user's runs in backend order, newest first. Refresh reloads manually; this page does not poll. Click a row to open its details.
- **My Strategies / Templates**: Remain placeholders.

Completed results show summary P&L and the actual backend trade fields. Monetary values are already in rupees; timestamps are displayed in the browser's local timezone. Open trades show a dash for null exit fields and P&L.

Routes: `/editor` (default), `/backtests`, `/backtests/:id`, `/strategies`, and `/templates`. Direct navigation works through Vite's SPA fallback.

Monaco and its worker are bundled locally, with no editor CDN dependency.
The DOMPurify override in `package.json` keeps Monaco's sanitizer dependency on a patched release.
For production hosting, serve `dist/`, configure an SPA fallback to `index.html` for client-side routes, and route `/api` to the Go server. The Vite development proxy is not included in the production build.
