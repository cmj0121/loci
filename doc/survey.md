# Loci survey

English | [繁體中文](survey.zh-TW.md)

After the gates closed Loci as a code-lookup tool ([Measurement](measurement.md)), the same
question was asked of every other kind of memory an agent could be given: does it save tokens on
a real team repository? Each section states one conclusion; the numbers behind it are in
`doc/results/`. All measurements ran on one large repository worked on by many people and
coding agents; it is not named here.

## 1. Agents rarely re-explore, so session memory has little to save

Exploration is most of what agents read, but almost all of it is first-time reading for the task
at hand. Re-reading an unchanged file in a later session is about 3% of exploration, and a
memory captured from past sessions is mostly noise about the tool environment. A memory of past
sessions has little to cut. [Results](results/survey-1-exploration.md)
