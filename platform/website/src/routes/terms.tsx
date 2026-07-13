import { createFileRoute } from "@tanstack/react-router"
import { BookOpen } from "lucide-react"
import * as React from "react"

import { PageHeader } from "@/components/layout/page-header"
import { PageShell } from "@/components/layout/page-shell"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { type GlossaryTerm, glossary } from "@/lib/glossary"

export const Route = createFileRoute("/terms")({
  component: GlossaryRoute,
})

type CategoryMeta = {
  label: string
  blurb: string
}

const CATEGORY_ORDER: GlossaryTerm["category"][] = [
  "identity",
  "transport",
  "state",
  "telemetry",
]

const CATEGORY_META: Record<GlossaryTerm["category"], CategoryMeta> = {
  identity: {
    label: "Identity",
    blurb: "Who and what the gateway trusts.",
  },
  transport: {
    label: "Transport & trust",
    blurb:
      "How agents reach the gateway and what the UI is allowed to believe.",
  },
  state: {
    label: "State",
    blurb: "What online, offline, and the timestamps actually mean.",
  },
  telemetry: {
    label: "Telemetry",
    blurb: "Agent-reported fields that are not trust-bearing.",
  },
}

function GlossaryRoute() {
  const grouped = React.useMemo(() => {
    const byCategory = new Map<GlossaryTerm["category"], GlossaryTerm[]>()
    for (const term of glossary) {
      const bucket = byCategory.get(term.category) ?? []
      bucket.push(term)
      byCategory.set(term.category, bucket)
    }
    return CATEGORY_ORDER.map((category) => ({
      category,
      meta: CATEGORY_META[category],
      terms: byCategory.get(category) ?? [],
    })).filter((group) => group.terms.length > 0)
  }, [])

  return (
    <PageShell>
      <PageHeader
        kicker={<BookOpen className="size-5 text-muted-foreground" />}
        title="Glossary"
        description="How this UI labels agents, identity, and telemetry. Trust sources are explicit."
      />

      <div className="flex flex-col gap-8">
        {grouped.map((group) => (
          <section key={group.category} className="flex flex-col gap-3">
            <div>
              <h2 className="text-lg font-semibold tracking-normal">
                {group.meta.label}
              </h2>
              <p className="text-sm text-muted-foreground">
                {group.meta.blurb}
              </p>
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              {group.terms.map((term) => (
                <TermCard key={term.slug} term={term} />
              ))}
            </div>
          </section>
        ))}
      </div>
    </PageShell>
  )
}

function TermCard({ term }: { term: GlossaryTerm }) {
  return (
    <Card id={term.slug} className="scroll-mt-24">
      <CardHeader>
        <CardTitle>{term.term}</CardTitle>
        <CardDescription>{term.summary}</CardDescription>
      </CardHeader>
      <CardContent className="text-sm text-muted-foreground">
        {term.detail}
      </CardContent>
    </Card>
  )
}
