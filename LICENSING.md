# Licensing and the Contributor License Agreement

Margince is source-available, not open source. The code is public and free to read, run and modify. Production use is free up to three seats, and from the fourth seat a commercial subscription applies. The limit applies to each release whose LICENSE states it; a copy obtained under an earlier LICENSE keeps the terms of that LICENSE. Nobody may host Margince as a service for others without a partner agreement. The full terms are in [LICENSE](LICENSE).

<!-- prose:allow bold counsel's wording; the conversion is stated as legal approved it -->
**Every release converts to the Apache License 2.0 no later than two years after it ships.** The LICENSE file of each release states its Change Date, the day that version becomes fully open source, permanently, for everyone. We describe Margince as "source-available, becomes Apache 2.0 in two years", and we do not describe it as open source before that point.

## Purpose of the agreement

Before we can merge your pull request, we need you to accept our [Contributor License Agreement](CLA.md). A bot will prompt you on your first pull request with a link to the CLA and a sentence to post as a comment. Posting that sentence records your acceptance, and it covers everything you contribute afterwards.

<!-- prose:allow sentence counsel's wording, kept verbatim -->
We ask for this because of how Margince is licensed. We sell commercial subscriptions, we license hosting partners, and we convert each release to Apache 2.0 on schedule. Every one of those requires us to hold licensing rights over the whole codebase, including the parts other people wrote. A Developer Certificate of Origin would confirm that you wrote your code, but it would not give us those rights, and a single contribution without them would leave part of the product that we cannot sell, cannot relicense and cannot convert.

## Effect of the agreement

The CLA is a licence, not a transfer, so you keep the copyright in your contribution and remain free to use, publish and relicense it anywhere else on any terms you choose.

<!-- prose:allow sentence counsel's wording; it states the conversion as policy, not promise -->
In return, we may license your contribution to others on any terms, including commercial ones. That grant is what makes the paid tiers and the scheduled Apache conversion possible, and it also means we are not contractually limited to the licence we use today. We intend every release to convert no later than two years after it ships, and we state it here as published policy so that you can see what it is: a policy, visible in the Change Date in each release's LICENSE file, rather than an individual promise to each contributor.

Your contribution comes as is. You give no warranty that it works, and you carry no liability for it.

## Before you open a pull request

Your contribution must be your own work, apart from AI-assisted parts you declare as described below. If it contains code written by someone else, or code under a different licence, say so clearly in the pull request.

If you used AI, the "Assisted" or "Generated" note described in ["Say how you used AI"](CONTRIBUTING.md#say-how-you-used-ai) is the declaration the CLA refers to. Under Clause 6.5 of the CLA you also confirm that you reviewed the AI output, that the tool's terms allow it to be contributed, and that to your knowledge it does not reproduce someone else's code. AI tools can reproduce code from their training data, so give AI output the same licence check you would give code copied from anywhere else.

Dependencies must be under a permissive licence, or under a file-level copyleft licence such as MPL-2.0 used as a separate package. Changes to files under such a licence are released under that licence, so do not copy those files into Margince's own code or patch them with anything that should stay under Margince's terms. GPL, AGPL, SSPL and similar licences are not accepted anywhere in the dependency tree, including packages pulled in indirectly, because they would prevent us from licensing the product as described above. LGPL needs prior approval from legal@gradion.com, because Margince ships as statically linked Go binaries. A licence check in the build applies this rule to the whole dependency tree.

If you are contributing as part of your job, check that your employer is comfortable with it. Your employer may accept the CLA on behalf of its staff under Clause 7 by sending a signed copy to legal@gradion.com. You then accept in the pull request in the usual way and say in the pull request which company you contribute for.

We ask every contributor to accept the CLA, including for small fixes and typo corrections. This reflects nothing about the size of the contribution. A record that carries exceptions does not establish a complete chain of rights, which is the only use the record has.
