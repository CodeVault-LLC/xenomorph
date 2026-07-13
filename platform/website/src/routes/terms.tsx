import { createFileRoute } from "@tanstack/react-router"
import {
  BookOpen,
  Network,
  Search,
  ShieldCheck,
  Waypoints,
  X,
} from "lucide-react"
import * as React from "react"

import { PageHeader } from "@/components/layout/page-header"
import { PageShell } from "@/components/layout/page-shell"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import {
  type GlossaryCategory,
  type GlossarySource,
  type GlossaryTerm,
  glossary,
} from "@/lib/glossary"
import { cn } from "@/lib/utils"

export const Route = createFileRoute("/terms")({
  component: GlossaryRoute,
})

type CategoryMeta = {
  label: string
  description: string
  icon: React.ComponentType<{ className?: string }>
}

const CATEGORY_ORDER: GlossaryCategory[] = [
  "transport",
  "identity",
  "state",
  "telemetry",
]

const CATEGORY_META: Record<GlossaryCategory, CategoryMeta> = {
  transport: {
    label: "Trust & transport",
    description: "Controls that establish authenticated gateway state.",
    icon: ShieldCheck,
  },
  identity: {
    label: "Identity & provenance",
    description:
      "Attributes that distinguish an agent record from reported labels.",
    icon: Waypoints,
  },
  state: {
    label: "Connection state",
    description: "Gateway observations that describe liveness and recency.",
    icon: Network,
  },
  telemetry: {
    label: "Reported telemetry",
    description: "Useful operational context that is not trust-bearing.",
    icon: BookOpen,
  },
}

function GlossaryRoute() {
  const [query, setQuery] = React.useState("")
  const normalizedQuery = query.trim().toLowerCase()
  const grouped = React.useMemo(() => {
    const matches = glossary.filter((term) => {
      if (!normalizedQuery) {
        return true
      }

      return [term.term, term.summary, term.detail, term.source]
        .join(" ")
        .toLowerCase()
        .includes(normalizedQuery)
    })

    return CATEGORY_ORDER.map((category) => ({
      category,
      meta: CATEGORY_META[category],
      terms: matches.filter((term) => term.category === category),
    })).filter((group) => group.terms.length > 0)
  }, [normalizedQuery])

  const resultCount = grouped.reduce(
    (count, group) => count + group.terms.length,
    0
  )

  return (
    <PageShell>
      <PageHeader
        kicker={<BookOpen className="size-5 text-muted-foreground" />}
        title="Operational reference"
        description="Authoritative meaning for system state, identity provenance, and agent-reported data. Terms are included only when they affect interpretation or action."
      />

      <div className="grid gap-5 xl:grid-cols-[240px_minmax(0,1fr)]">
        <aside className="flex flex-col gap-4 xl:sticky xl:top-20 xl:self-start">
          <Card>
            <CardHeader>
              <CardTitle>Reference index</CardTitle>
              <CardDescription>
                {glossary.length} operational terms across four domains.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-1 p-2">
              {CATEGORY_ORDER.map((category) => {
                const meta = CATEGORY_META[category]
                const Icon = meta.icon
                const count = glossary.filter(
                  (term) => term.category === category
                ).length

                return (
                  <Button
                    key={category}
                    variant="ghost"
                    render={<a href={`#${category}`} />}
                    className="w-full justify-start text-muted-foreground"
                  >
                    <Icon data-icon="inline-start" />
                    {meta.label}
                    <span className="ml-auto text-xs tabular-nums">
                      {count}
                    </span>
                  </Button>
                )
              })}
            </CardContent>
          </Card>

          <Alert>
            <ShieldCheck />
            <AlertTitle>Interpretation rule</AlertTitle>
            <AlertDescription>
              Gateway-authored fields identify records and describe connection
              state. Agent-authored data provides operational context only.
            </AlertDescription>
          </Alert>
        </aside>

        <div className="flex min-w-0 flex-col gap-6">
          <section
            aria-labelledby="reference-search-title"
            className="flex flex-col gap-3"
          >
            <div className="flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
              <div>
                <h2
                  id="reference-search-title"
                  className="text-base font-semibold"
                >
                  Find a term
                </h2>
                <p className="text-sm text-muted-foreground">
                  Search title, operational meaning, or provenance.
                </p>
              </div>
              <p className="text-sm text-muted-foreground" aria-live="polite">
                {resultCount} {resultCount === 1 ? "result" : "results"}
              </p>
            </div>
            <div className="flex gap-2">
              <div className="relative min-w-0 flex-1">
                <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Search the operational reference"
                  aria-label="Search the operational reference"
                  className="pl-9"
                />
              </div>
              {query ? (
                <Button variant="outline" onClick={() => setQuery("")}>
                  <X data-icon="inline-start" />
                  Clear
                </Button>
              ) : null}
            </div>
          </section>

          {grouped.length ? (
            grouped.map((group) => (
              <TermGroup key={group.category} {...group} />
            ))
          ) : (
            <Card>
              <CardContent>
                <Empty>
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <Search />
                    </EmptyMedia>
                    <EmptyTitle>No matching terms</EmptyTitle>
                    <EmptyDescription>
                      Try a broader search or clear the current query.
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </PageShell>
  )
}

function TermGroup({
  category,
  meta,
  terms,
}: {
  category: GlossaryCategory
  meta: CategoryMeta
  terms: GlossaryTerm[]
}) {
  const Icon = meta.icon

  return (
    <section id={category} className="scroll-mt-24">
      <div className="flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-card">
          <Icon className="size-4 text-muted-foreground" />
        </span>
        <div>
          <h2 className="text-lg font-semibold tracking-normal">
            {meta.label}
          </h2>
          <p className="mt-0.5 text-sm text-muted-foreground">
            {meta.description}
          </p>
        </div>
      </div>

      <div className="mt-4 flex flex-col gap-3">
        {terms.map((term) => (
          <TermCard key={term.slug} term={term} />
        ))}
      </div>
    </section>
  )
}

function TermCard({ term }: { term: GlossaryTerm }) {
  return (
    <Card id={term.slug} className="scroll-mt-24">
      <CardHeader className="gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <CardTitle>{term.term}</CardTitle>
          <CardDescription className="mt-1.5 max-w-3xl">
            {term.summary}
          </CardDescription>
        </div>
        <SourceBadge source={term.source} />
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <Separator />
        <div className="grid gap-1.5">
          <div className="text-xs font-medium text-muted-foreground uppercase">
            Operational meaning
          </div>
          <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
            {term.detail}
          </p>
        </div>
      </CardContent>
    </Card>
  )
}

function SourceBadge({ source }: { source: GlossarySource }) {
  return (
    <Badge
      variant={source === "Agent-authored" ? "secondary" : "outline"}
      className={cn("shrink-0", source === "Gateway-authored" && "bg-card")}
    >
      {source}
    </Badge>
  )
}
