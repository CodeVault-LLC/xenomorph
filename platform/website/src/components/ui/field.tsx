import * as React from "react"

import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

function FieldGroup(props: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="field-group"
      {...props}
      className={cn("flex w-full flex-col gap-5", props.className)}
    />
  )
}

function Field(props: React.ComponentProps<"div">) {
  return (
    <div
      role="group"
      data-slot="field"
      {...props}
      className={cn(
        "group/field flex w-full flex-col gap-2 data-[invalid=true]:text-destructive",
        props.className
      )}
    />
  )
}

function FieldLabel(props: React.ComponentProps<typeof Label>) {
  return <Label data-slot="field-label" {...props} />
}

function FieldDescription(props: React.ComponentProps<"p">) {
  return (
    <p
      data-slot="field-description"
      {...props}
      className={cn(
        "text-sm leading-normal text-muted-foreground",
        props.className
      )}
    />
  )
}

function FieldError(props: React.ComponentProps<"p">) {
  if (!props.children) return null
  return (
    <p
      role="alert"
      data-slot="field-error"
      {...props}
      className={cn("text-sm text-destructive", props.className)}
    />
  )
}

export { Field, FieldDescription, FieldError, FieldGroup, FieldLabel }
