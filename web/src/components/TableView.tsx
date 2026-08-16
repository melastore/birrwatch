interface Props {
  summary: string;
  columns: string[];
  rows: Array<Array<string | number>>;
}

/**
 * The WCAG-clean twin of a chart. Every chart on the page has one, so no value
 * is reachable only by hovering and no meaning rests on color alone.
 */
export default function TableView({ summary, columns, rows }: Props) {
  return (
    <details className="table-view">
      <summary>{summary}</summary>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              {columns.map((c) => (
                <th key={c} scope="col">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={String(r[0])}>
                {r.map((cell, i) => (
                  <td key={i}>{cell}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </details>
  );
}
