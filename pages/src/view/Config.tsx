import { useState } from 'react'

export default function Config() {
  const config = useState()

  const buildSchema = (schema: any) => {
    return [<div></div>]
  }

  return (
    <div>
      <form>{...buildSchema(schema)}</form>
    </div>
  )
}
