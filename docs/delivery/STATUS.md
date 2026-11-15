# Статус поставки

Обновлено: 2026-11-15.

- Прочитаны оба исходных чата ревьюеров и актуальные открытые GitHub issues #1–4.
- Продуктовое решение «план/факт» подтверждено пользователем.
- Рабочая копия подготовлена в папке проекта; чужие dirty-правки сохранены коммитом 788419e.
- Архитектурная ветка release/A-1: 1a16e80, миграции и system_code SALARY; [PR #5](https://github.com/Guulik/dengovod/pull/5) слит в main, merge 1191346. Go tests/build прошли в отдельной копии итоговой ветки.
- Backend reviewer выявил два дефекта миграций (CRLF checksum и wildcard SQLite-prefix); оба исправлены, повторное ревью: No findings.
- Пользователь явно разрешил самостоятельно сливать проверенные PR по этому бэклогу в main после успешных тестов и повторного ревью. Первоначальный отказ автоматической проверки разрешений снят новым явным разрешением.
- Функциональная ветка общей рабочей копии: release/DV-1, основана на A-1.
- Составлены SPEC.md и APPROVED CONTRACT.md; design findings обоих ревьюеров обработаны.
- Созданы исполнители backend/frontend и независимые QA business/e2e. Реализация завершена, код зафиксирован: frontend 9d839a5, backend c42b473, независимые тесты c862f96.
- [Функциональный PR #6](https://github.com/Guulik/dengovod/pull/6) опубликован; повторные backend/frontend ревью по точным коммитам завершены: No findings.
- После ревью: frontend 96ed958 (полночь и точный момент факта), 6fa8db7 (Today на мобильном); повторное frontend/CSS ревью — No findings. Полный frontend набор теперь 67 тестов, новые два e2e Today прошли, включая 320/390px.
- Backend fixes a3d2452 (bounded/sparse расчёт и aggregate legacy validation до commit v3) и 3e54780 (пропуск перекрытого исторического резерва) прошли повторное review: No findings.
- Локально прошли 37 независимых бизнес-сценариев, 67 frontend unit/component tests, 32 базовых desktop/mobile e2e и 2 новые адресные регрессии Today. Go tests/vet/build и frontend lint/build/e2e typecheck прошли.
- [CI run 43715370852](https://github.com/Guulik/dengovod/actions/runs/43715370852): backend с race detector, frontend и e2e на Linux — PASS.
- PR #6 содержит Closes для всех четырёх issues; актуальное состояние слияния и закрытия задач отражается в GitHub. Финальные проверки полного набора 34 e2e, frontend и backend с race detector — в checks PR #6.
- Итоговый отчёт: REPORT.md; подробные QA-результаты: ../qa/business-report.md и ../qa/e2e-report.md.
