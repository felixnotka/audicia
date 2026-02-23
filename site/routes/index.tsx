import { Head } from "fresh/runtime";
import { define } from "../utils.ts";
import Hero from "../components/landing/Hero.tsx";
import ComplianceScore from "../components/landing/ComplianceScore.tsx";
import Problem from "../components/landing/Problem.tsx";
import HowItWorks from "../components/landing/HowItWorks.tsx";
import Features from "../components/landing/Features.tsx";
import Compliance from "../components/landing/Compliance.tsx";
import Comparison from "../components/landing/Comparison.tsx";
import QuickStart from "../components/landing/QuickStart.tsx";
import FAQ from "../components/landing/FAQ.tsx";
import OpenSource from "../components/landing/OpenSource.tsx";

const faqStructuredData = {
  "@context": "https://schema.org",
  "@type": "FAQPage",
  mainEntity: [
    {
      "@type": "Question",
      name: "Does Audicia work on managed Kubernetes (EKS, GKE, AKS)?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "Managed Kubernetes services don't expose apiserver flags or audit log files directly. Audicia currently works with self-managed clusters where you control the audit log path or can configure a webhook backend. Cloud-specific log ingestors are on the roadmap.",
      },
    },
    {
      "@type": "Question",
      name: "Will Audicia automatically apply RBAC policies to my cluster?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "No. Audicia never auto-applies policies. By design, it generates recommendations. A human or a reviewed pipeline applies them.",
      },
    },
    {
      "@type": "Question",
      name: "How is Audicia different from audit2rbac?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "audit2rbac is a CLI tool for one-time analysis that re-reads the entire audit log on every run. Audicia is an operator that runs continuously with state management, checkpoint/resume, event normalization, and compliance scoring built in.",
      },
    },
    {
      "@type": "Question",
      name: "Can Audicia work alongside OPA/Gatekeeper?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "Yes, they're complementary. Audicia generates policy, OPA/Gatekeeper enforces policy. The ideal stack uses both.",
      },
    },
    {
      "@type": "Question",
      name: "Is there a paid version?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "No. Audicia is Apache 2.0 licensed. The full operator, both ingestion modes, compliance scoring, and the complete Helm chart ship free. There is no paid tier, no enterprise edition, no feature gating.",
      },
    },
  ],
};

export default define.page(function Home() {
  return (
    <div>
      <Head>
        <meta property="og:title" content="Stop Writing RBAC by Hand" />
        <meta
          property="og:description"
          content="Audicia observes your Kubernetes audit logs and generates the minimal RBAC policies your workloads actually need. Open source. Apache 2.0."
        />
        <script
          type="application/ld+json"
          // deno-lint-ignore react-no-danger
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              "@context": "https://schema.org",
              "@type": "SoftwareApplication",
              name: "Audicia",
              applicationCategory: "DeveloperApplication",
              operatingSystem: "Kubernetes",
              description:
                "Kubernetes Operator that watches audit logs and generates least-privilege RBAC policies automatically.",
              url: "https://audicia.io/",
              license: "https://www.apache.org/licenses/LICENSE-2.0",
              isAccessibleForFree: true,
              codeRepository: "https://github.com/felixnotka/audicia",
              author: {
                "@type": "Person",
                name: "Felix Notka",
              },
            }),
          }}
        />
        <script
          type="application/ld+json"
          // deno-lint-ignore react-no-danger
          dangerouslySetInnerHTML={{
            __html: JSON.stringify(faqStructuredData),
          }}
        />
      </Head>
      <main id="main">
        <Hero />
        <ComplianceScore />
        <Problem />
        <HowItWorks />
        <Features />
        <Compliance />
        <Comparison />
        <QuickStart />
        <FAQ />
        <OpenSource />
      </main>
    </div>
  );
});
