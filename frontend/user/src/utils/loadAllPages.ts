export interface PageResult<T> {
    items: T[]
    totalPages: number
}

export async function loadAllPages<T>(
    fetchPage: (page: number, pageSize: number) => Promise<PageResult<T>>,
    pageSize = 200,
): Promise<T[]> {
    const firstPage = await fetchPage(1, pageSize)
    const totalPages = Math.max(1, Math.trunc(Number(firstPage.totalPages) || 1))
    const items = [...firstPage.items]

    for (let page = 2; page <= totalPages; page += 1) {
        const nextPage = await fetchPage(page, pageSize)
        items.push(...nextPage.items)
    }

    return items
}
