<!-- prose:plain -->
<!-- describes: scripts/e2e-llm.sh e2e/llm/check.py e2e/llm/judge.py e2e/llm/endstate.py e2e/llm/drive.py e2e/llm/clean.py e2e/llm/scenarios -->
# How we test your AI assistant

Your AI assistant is an AI tool a user connects to Margince over MCP, such as Claude Code or Codex.
Here, in plain words, is how we find out which daily jobs it does well. The results are on
[Your AI assistant: test results](../reference/mcp-tool-coverage.md). The page for both ways AI works
with Margince is [which-ai-you-can-use.md](../reference/which-ai-you-can-use.md). To run the tests, see
[test-the-mcp-surface-end-to-end.md](../how-to/test-the-mcp-surface-end-to-end.md). The AI inside the
product is tested another way: [How we test Margince AI](ai-certification.md).

## What a job is

A **job** is one task a user gives an assistant, in the words a user would type. Logging a call, getting
ready for a meeting and cleaning up a deal room are jobs. Each job is one file under
`e2e/llm/scenarios/`. It holds the prompt, the test data the job needs, and what a good answer must do.
The `title:` line of the file is the name the result page shows.

A job tests the whole task, not one step of it. The assistant gets a test company full of records,
the same tools a user's assistant gets, and nothing else. It decides on its own which tools to call
and in which order, as it would for a user.

## How many tries

An assistant does not answer the same way twice, so each job runs up to three times. A job passes when
at least two tries do the job right. The test stops after two tries when both agree. Before each try
the test company goes back to the same start, so one try cannot change the next.

A job that passes is **done reliably**. An assistant that does every job we tried reliably reads
**Yes** on the result page. One that does at least 80 in every 100 reads **Mostly**.

## What is checked

A job checks a try in up to four ways. The try passes only when every check the job names holds.

1. **What it called.** A job names the tools a right answer must call, and the calls it must not make.
   A call counts only when it worked. The test company cannot send mail, so there a refused send
   counts too.
2. **What it said.** The final answer must name the facts of the job. Nothing the assistant wrote on
   the way may claim what did not happen.
3. **What it left behind.** After the try, the test reads the records back and compares them with the
   state the job names.
4. **What a reader would judge.** A second model reads the final answer and says yes or no to each
   point a rule cannot check. When it says no, it is asked two more times, and two answers of three decide.

A mail the user did not ask to send is a call the job refuses. "Sent", for a mail that could not go, is
a claim the job refuses. A company whose stage did not change as asked is a state the job names.

## Who judges

The judge is a small Claude model, `claude-haiku-5-5`, the same one for every assistant. Before it
judges any assistant, it is tested itself against answers that our team graded by hand. The result
page prints how often each judge model agreed with those grades. So a reader sees the error of the
judge next to the scores it gave. When the assistant under test is a Claude model too, its result
files record that a model of its own family judged it.

## How an assistant is connected

An assistant can be reached three ways, and the result page says which one each row used.

- **Through its own app**, such as the Claude Code or Codex command line. That is how a user meets it,
  with the instructions of its own maker. Each try starts in an empty folder. It holds no settings,
  files or memory of ours, so the assistant knows only what the server tells it.
- **Through a host we control**, for Claude through the Claude Agent SDK. It is the
  same Claude Code app, and the host answers the questions the server asks in the chat, such as the
  yes for a bulk change, as the job says the user answers. The command line cannot answer them, so a
  job that needs such an answer is measured this way.
- **Through a neutral bridge**, which gives every model the same tools, the same instructions and the
  same limit of turns. Only this way compares one model with another on the same footing.

## When a try does not count

A try that could not have passed is not a failure of the assistant. A refused key is one such case.
A tool the assistant could not see, or a record the test could not read back, is another. Such a try
stops the run and scores nothing. The run reports it as a fault of the test, so it never reads as a
fault of the product.

## What the tests cannot show

- **Only the jobs we wrote.** A tool that no job asks for is not tested, which is not the same as
  broken. The result page lists the tools no job needs yet.
- **Only the assistants we ran.** An assistant missing from the table may work well. We do not know.
- **A test company, not yours.** Your records, your words and your size of data can change how an
  assistant does.
- **A moment in time.** The tests cost real model use and run when someone starts them. So a result
  can be older than the assistant or the product, and a new version of either can do better or worse.
  Each result records what it measured: the job, the tool texts, the instructions, the judge and the
  test itself. When one of them changes, the result is out of date, and a change that causes it is told so.
- **The rights of the user.** The tests show what an assistant does well. What it may do is a separate
  rule: it can do what the user behind it could do, and nothing more
  ([agent-surface.md](agent-surface.md)).
