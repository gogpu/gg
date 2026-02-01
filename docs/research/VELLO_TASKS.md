# Vello AA Tasks

## Текущая задача
**ARCH-007**: Fix Vello tile-based AA backdrop propagation

---

## Все задачи агента (из TaskList)

| ID | Status | Task |
|----|--------|------|
| #4 | **in_progress** | **v0.22.0: Vello AA + Polish (ARCH-005, ARCH-006, ARCH-007)** |
| #56 | **in_progress** | ARCH-007: Deep study and fix vello_tiles.go algorithm |
| #13 | pending | TEST-001: Add tests for core/ package (70%+ coverage) |
| #15 | pending | ARCH-008: gg pluggable backends research |
| #16 | pending | ARCH-009: gogpu pluggable backends research |
| #27 | pending | INT-005: RenderTo method for gogpu |
| #31 | pending | RESEARCH-001: Investigate Gio UI patterns for gogpu ecosystem |
| #32 | pending | NAGA-001: Integrate gputypes.TextureFormat for storage textures |
| #35 | pending | PERF-001: Replace QueueWaitIdle with staging belt for WriteBuffer |

---

## Задачи для ARCH-007 fix

### 1. Понять как Rust обрабатывает формы внутри тайла
- [ ] Форма начинается в Y=10 (тайл покрывает Y=0-15)
- [ ] topEdge = false для i==0 (0 != 0.625)
- [ ] Как backdrop попадает в tile(1,0) для Y=10-15?

### 2. Проверить yEdge в деталях
- [ ] Когда segment.y_edge устанавливается (path_tiling.rs:136,142)
- [ ] Как yEdge contribution работает в fine.wgsl (GPU)
- [ ] Почему в Go yEdge не компенсирует отсутствие backdrop?

### 3. Сравнить с AnalyticFiller
- [ ] AnalyticFiller работает правильно — что он делает по-другому?

### 4. Тесты
- [ ] Прямоугольник на границе тайла (должен работать)
- [ ] Прямоугольник ВНУТРИ тайла (проблема!)
- [ ] Диагональ

---

## Тестовые команды

```bash
cd D:/projects/gogpu/gg
go test -v -run "TestVelloCompare" ./backend/native/...
go test -v -run "TestVelloVisual" ./backend/native/...
go test -v -run "TestVelloTileDebug" ./backend/native/...
```

---

## Документация

- **Находки и анализ:** [VELLO_BACKDROP_PROBLEM.md](./VELLO_BACKDROP_PROBLEM.md)
- **Статус проекта:** [.claude/STATUS.md](../../.claude/STATUS.md)
