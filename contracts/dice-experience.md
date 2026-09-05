# Dice experience (browser-only)

`/games/dice` is a signed-in, clearly labelled rules simulation. It does not create wagers, server rounds, wallet transactions, rewards, or API quota changes. It is not completion of IS-06 settlement or Provably Fair.

- Three independent six-sided dice. Non-triple totals 4–10 resolve SMALL; 11–17 resolve BIG. All six triples resolve TRIPLE regardless of the sum.
- The 216 equally likely outcomes contain 105 SMALL, 105 BIG and 6 TRIPLE. A guess only changes the local result label; there are no stakes or prizes.
- Browser `crypto.getRandomValues` supplies bytes. Reject 252–255 before modulo-six mapping. Random-source errors produce no new result; no weaker fallback.
- The only result-generating action is the explicit “模拟掷骰” button. Changing the choice, navigation and reload never generate a result.
- Up to 20 local records are stored in `sessionStorage` under `momiao.dice.experience.v1.<userID>`. Only dice, choice, local timestamp and display fields are present; no credentials or seeds. On reload, validate the data and recompute the outcome.
- Local records are user-editable and are not authoritative receipts. They are not synced between devices or submitted to the server. Account/session changes remount the page. Blocked storage degrades to in-page history with a notice; corrupt records are ignored.
- There is no game API or database migration. Existing wallet and native APIs are unchanged.

Checks: exhaustive outcome classification; rejected-byte mapping; random/storage failure; explicit action; account isolation; 20-record retention; clear/reload; authenticated route and no wallet/game requests. Existing frontend and portal tests cover login/logout and route serving.
